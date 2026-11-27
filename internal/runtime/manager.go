package runtime

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
	"github.com/migueel15/dory/internal/harness/procpool"
	"github.com/migueel15/dory/internal/i18n"
	"github.com/migueel15/dory/internal/nodestate"
	"github.com/migueel15/dory/internal/outbox"
	"github.com/migueel15/dory/internal/workspace"
)

var (
	ErrNotFound = apperr.New(apperr.NotFound, "thread not found")
	ErrRemote   = apperr.New(apperr.Remote, "thread runs on another node")
	ErrProvider = harness.ErrProvider
	ErrBusy     = apperr.New(apperr.Busy, "wait for the running turn to end before switching provider")
)

// Update is a hint pushed to the global stream when a thread changes: a
// message or a new status. It is lossy; the record of events of each thread
// (ThreadEvent) is what a thread's own stream is built on.
type Update struct {
	ThreadID string   `json:"thread_id"`
	Message  *Message `json:"message,omitempty"`
	Status   string   `json:"status,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// Queued is a message waiting for the running turn to end.
type Queued struct {
	RunID string       `json:"run_id"`
	Text  string       `json:"text"`
	Files []Attachment `json:"files,omitempty"`
	// Sender is the thread whose agent sent the message; empty for the user.
	Sender string `json:"sender_thread_id,omitempty"`
}

// Manager owns the threads of this node: it starts turns, keeps their
// messages and tells subscribers what changed.
type Manager struct {
	adoptMu  sync.Mutex
	db       *sql.DB
	state    *nodestate.Store
	store    *data.Store
	node     string
	dataDir  string
	registry *harness.Registry
	pool     *procpool.Pool
	// MCPURL is the local endpoint. MCPToken is the node secret used to derive thread credentials.
	MCPURL, MCPToken string

	// mu guards the subscriptions and the cached harness state.
	mu     sync.Mutex
	subs   map[chan Update]string
	evSubs map[*eventSub]struct{}
	// evMu orders the recording and the live sending of events.
	evMu sync.Mutex
	// writing holds, by thread and item, the texts being written: a client
	// that connects gets them whole. Guarded by evMu.
	writing map[string]map[string]Message
	// runners holds the actor of each thread with work or a queue; nothing
	// else about a thread's turn lives in the Manager.
	runnersMu sync.Mutex
	runners   map[string]*threadRunner
	// runnerIdle is how long an actor with nothing to do stays alive.
	runnerIdle time.Duration
	usage      *nodestate.Value[map[string][]Window]
	// Outbox carries what this node owes other nodes, such as the outcome of
	// a subagent turn to its parent; set at start-up.
	Outbox *outbox.Outbox
	// OnStateChange runs after a local thread state change is stored.
	OnStateChange func(threadID string)
	// detected caches what Providers found.
	detected []Status
	// catalogs keeps the last model list each harness reported, so a slow or
	// failed catalog read never falls back to the short built-in list.
	catalogs   map[string][]Model
	detectedAt time.Time
	// detecting is closed when the running detection ends; nil when none
	// runs. detectGen counts invalidations, so an older result is dropped.
	detecting chan struct{}
	detectGen int
	// OnProvidersChange runs when a background detection finds something
	// different from what was served.
	OnProvidersChange func()
}

func NewManager(db *sql.DB, store *data.Store, node, dataDir string, state *nodestate.Store, pool *procpool.Pool, providers ...Provider) *Manager {
	if pool == nil {
		pool = procpool.New(nil)
	}
	m := &Manager{db: db, state: state, store: store, node: node, dataDir: dataDir, pool: pool, registry: harness.NewRegistry(state, dataDir, pool, providers...),
		subs: map[chan Update]string{}, evSubs: map[*eventSub]struct{}{}, runners: map[string]*threadRunner{}, runnerIdle: 10 * time.Minute}
	m.usage = nodestate.NewValue[map[string][]Window](state, nodestate.KeyUsage, nil)
	if err := m.usage.Load(context.Background()); err != nil {
		log.Printf("runtime: load usage: %v", err)
	}
	return m
}

// WatchUsage asks every provider for its usage now and then every interval,
// until ctx ends. A failed query keeps the last known windows.
func (m *Manager) WatchUsage(ctx context.Context, every time.Duration) {
	for {
		m.RefreshUsage(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// RefreshUsage asks every provider for its usage at once.
func (m *Manager) RefreshUsage(ctx context.Context) {
	var wg sync.WaitGroup
	for _, p := range m.registry.All() {
		id := p.Info().ID
		if p.Usage == nil || !p.Info().Capabilities.UsageReport || !m.Enabled(id) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			windows, err := p.Usage.Usage(ctx)
			switch {
			case errors.Is(err, ErrNoUsage):
			case err != nil:
				log.Printf("runtime: %s usage: %v", id, err)
			case len(windows) > 0:
				m.saveUsage(ctx, id, windows)
			}
		}()
	}
	wg.Wait()
}

func (m *Manager) saveUsage(ctx context.Context, id string, windows []Window) {
	if err := m.usage.Update(ctx, func(usage *map[string][]Window) {
		if *usage == nil {
			*usage = make(map[string][]Window)
		}
		(*usage)[id] = windows
	}); err != nil {
		log.Printf("runtime: save usage: %v", err)
	}
}

// Recover marks as stopped the threads that were working when the node went
// down: their processes died with it.
func (m *Manager) Recover(ctx context.Context) error {
	m.StopArchivedServices()
	if err := m.dropRequests(ctx); err != nil {
		return err
	}
	m.loadQueues(ctx)
	threads, err := m.store.Threads().ListAll(ctx, data.Filter{Fields: map[string]any{"node_id": m.node, "status": "working"}})
	if err != nil {
		return err
	}
	for _, t := range threads {
		failure := i18n.Text("node.restarted")
		if err := m.store.SetThreadState(ctx, t.ID, "error", failure); err != nil {
			return err
		}
		m.save(ctx, Message{ID: newRunID(), ThreadID: t.ID, Role: "system", Code: "node.restarted", CreatedAt: now()})
	}
	return nil
}

// providersFresh is how long a detection counts as current. An older one is
// still served at once while a new detection runs in the background.
const providersFresh = time.Minute

// Providers reports the harnesses of this machine, with their session.
// Detecting runs each harness, which can take seconds, so the last answer is
// served at once and refreshed in the background when it is older than
// providersFresh. Only the first call, or one after invalidate, waits.
func (m *Manager) Providers(ctx context.Context) []Status {
	m.mu.Lock()
	out, stale := m.detected, time.Since(m.detectedAt) >= providersFresh
	var wait chan struct{}
	if out == nil || stale {
		wait = m.detectLocked()
	}
	m.mu.Unlock()
	if out == nil {
		select {
		case <-wait:
		case <-ctx.Done():
		}
		m.mu.Lock()
		out = m.detected
		m.mu.Unlock()
	}
	return m.withEnabled(out)
}

// withEnabled copies the list with the current switch of each harness, which
// changes without a new detection.
func (m *Manager) withEnabled(in []Status) []Status {
	out := make([]Status, len(in))
	for i, st := range in {
		st.Enabled = m.Enabled(st.ID)
		out[i] = st
	}
	return out
}

// detectLocked starts a detection unless one is running and returns the
// channel closed when it ends. m.mu must be held.
func (m *Manager) detectLocked() chan struct{} {
	if m.detecting != nil {
		return m.detecting
	}
	done := make(chan struct{})
	m.detecting = done
	gen := m.detectGen
	go func() {
		out := m.detect(context.Background())
		m.mu.Lock()
		changed := false
		// A detection started before invalidate may hold an old session.
		if gen == m.detectGen {
			before, _ := json.Marshal(m.detected)
			after, _ := json.Marshal(out)
			changed = m.detected != nil && string(before) != string(after)
			m.detected, m.detectedAt = out, time.Now()
		}
		m.detecting = nil
		m.mu.Unlock()
		close(done)
		if changed && m.OnProvidersChange != nil {
			m.OnProvidersChange()
		}
	}()
	return done
}

// detect runs every harness at once and keeps the registry order.
func (m *Manager) detect(ctx context.Context) []Status {
	home, _ := os.UserHomeDir()
	all := m.registry.All()
	out := make([]Status, len(all))
	var wg sync.WaitGroup
	for i, p := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := p.Info()
			st := p.Detect(ctx)
			st.ID, st.Driver, st.DriverName = info.ID, info.Driver, info.Name
			st.Models, st.Catalog = m.rememberCatalog(info.ID, st)
			st.Capabilities = info.Capabilities.Names()
			if p.Accounts != nil {
				st.Capabilities = append(st.Capabilities, "accounts")
				st.DefaultHome = p.Accounts.DefaultHome(home)
				st.ConfigEnv = p.Accounts.ConfigEnv()
			}
			if p.Commands != nil {
				st.Capabilities = append(st.Capabilities, "skills")
			}
			if p.Admin != nil && st.Installed {
				st.LoggedIn, st.Account = p.Admin.Session(ctx)
				st.Capabilities = append(st.Capabilities, "login", "update")
			} else if p.Admin != nil {
				st.Capabilities = append(st.Capabilities, "install")
			}
			out[i] = st
		}()
	}
	wg.Wait()
	return out
}

// invalidate forgets the cached harness state after a login, update, install
// or account change: the next call waits for a new detection.
func (m *Manager) invalidate() {
	m.mu.Lock()
	m.detected = nil
	m.detectGen++
	m.mu.Unlock()
}

// Usage returns the latest usage windows each provider reported.
func (m *Manager) Usage() map[string][]Window {
	usage := m.usage.Get()
	out := make(map[string][]Window, len(usage))
	now := time.Now()
	for k, v := range usage {
		if !m.Enabled(k) {
			continue
		}
		ws := make([]Window, 0, len(v))
		for _, w := range v {
			if !w.ResetsAt.IsZero() && w.ResetsAt.Before(now) {
				// Antigravity balances come from a snapshot. A reset does
				// not tell us how much was consumed in the new window.
				if m.registry.Capabilities(k).UsageSnapshot {
					continue
				}
				w.Utilization = 0
			}
			ws = append(ws, w)
		}
		out[k] = ws
	}
	return out
}

// Dir returns the working directory of a thread of this node.
func (m *Manager) Dir(ctx context.Context, id string) (string, error) {
	var dir string
	err := m.db.QueryRowContext(ctx, `SELECT worktree_path FROM thread_runtime WHERE thread_id = ?`, id).Scan(&dir)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return dir, err
}

func (m *Manager) local(ctx context.Context, id string) (data.Thread, error) {
	t, err := m.store.Threads().Get(ctx, id)
	if errors.Is(err, data.ErrNotFound) {
		return t, ErrNotFound
	}
	if err == nil && t.NodeID != m.node {
		return t, ErrRemote
	}
	return t, err
}

// Send starts a turn with text, or queues it while another turn runs.
func (m *Manager) Send(ctx context.Context, id, text string, files ...Attachment) error {
	return m.SendFrom(ctx, id, "", text, files...)
}

// SendFrom is Send for a message an agent of the sender thread writes.
func (m *Manager) SendFrom(ctx context.Context, id, sender, text string, files ...Attachment) error {
	return m.send(ctx, id, sender, text, false, files)
}

// SendSteer is Send from the user that, on a busy thread, stops the open turn
// and runs the message next instead of queueing it behind the turn.
func (m *Manager) SendSteer(ctx context.Context, id, text string, files ...Attachment) error {
	return m.send(ctx, id, "", text, true, files)
}

func (m *Manager) send(ctx context.Context, id, sender, text string, steer bool, files []Attachment) error {
	if strings.TrimSpace(text) == ClearCommand && len(files) == 0 {
		return m.Clear(ctx, id)
	}
	if _, err := m.local(ctx, id); err != nil {
		return err
	}
	return m.do(id, &command{kind: cmdSend, ctx: ctx, steer: steer, queued: Queued{RunID: newRunID(), Text: text, Files: files, Sender: sender}}).err
}

// ClearCommand is Dory's own /clear, the same for every harness.
const ClearCommand = "/clear"

// Clear forgets the harness session of a thread: its next turn starts a new
// one, because every runtime starts fresh without a session. The visible
// messages stay, after a separator.
func (m *Manager) Clear(ctx context.Context, id string) error {
	if _, err := m.local(ctx, id); err != nil {
		return err
	}
	return m.do(id, &command{kind: cmdClear, ctx: ctx}).err
}

func (m *Manager) clearSession(ctx context.Context, id string) error {
	t, err := m.local(ctx, id)
	if err != nil {
		return err
	}
	// Keep session_provider so no history is carried over as a handoff.
	if _, err := m.db.ExecContext(ctx, `UPDATE thread_runtime SET session_id = '', portable_history = 0 WHERE thread_id = ?`, id); err != nil {
		return err
	}
	m.pool.CloseProcess(id, t.Provider)
	m.save(ctx, Message{ID: newRunID(), ThreadID: id, Role: "clear", Code: "context.cleared", CreatedAt: now()})
	return nil
}

// Ask is Dory's ask_user: the agent's call waits until the user answers in
// the composer, as with the questions a harness asks natively, and gets the
// answers as its result. Leaving (the turn stops) withdraws the question.
func (m *Manager) Ask(ctx context.Context, id string, questions []Question) ([][]string, error) {
	if _, err := m.local(ctx, id); err != nil {
		return nil, err
	}
	var text []string
	for _, q := range questions {
		text = append(text, q.Question)
	}
	msg := Message{ID: newRunID(), ThreadID: id, Role: "question", Code: "question.title", Text: strings.Join(text, "\n"), Status: "waiting", CreatedAt: now(),
		Tool: &ToolCall{Kind: "question", Name: "ask_user", Questions: questions}}
	m.save(ctx, msg)
	w := m.do(id, &command{kind: cmdAsk, ctx: ctx})
	if w.err != nil {
		return nil, w.err
	}
	req := w.req
	m.request(ctx, id, Request{ID: req, Kind: "question", MessageID: msg.ID, Questions: questions})
	select {
	case a := <-w.answers:
		msg.Tool.Questions = kit.Answered(questions, a)
		msg.Status = "done"
		m.save(context.WithoutCancel(ctx), msg)
		return a, nil
	case <-ctx.Done():
		bg := context.WithoutCancel(ctx)
		m.do(id, &command{kind: cmdForgetAsk, ctx: bg, req: req})
		m.db.ExecContext(bg, `DELETE FROM thread_requests WHERE id = ?`, req)
		m.emitJSON(bg, id, EventRequestResolved, map[string]string{"id": req, "kind": "question"})
		return nil, ctx.Err()
	}
}

// Interrupt stops the running turn and drops the queue.
func (m *Manager) Interrupt(ctx context.Context, id string) error {
	if _, err := m.local(ctx, id); err != nil {
		return err
	}
	return m.do(id, &command{kind: cmdInterrupt, ctx: ctx}).err
}

// view is what the actor of a thread last published; a thread without actor
// is idle with an empty queue.
func (m *Manager) view(id string) runnerView {
	if r := m.lookup(id); r != nil {
		return *r.view.Load()
	}
	return runnerView{}
}

// Running reports whether a turn of the thread is in progress here.
func (m *Manager) Running(id string) bool { return m.view(id).State.busy() }

// Active counts the turns in progress on this node.
func (m *Manager) Active() int {
	m.runnersMu.Lock()
	defer m.runnersMu.Unlock()
	n := 0
	for _, r := range m.runners {
		if r.view.Load().State.busy() {
			n++
		}
	}
	return n
}

// Queue returns the messages waiting in a thread.
func (m *Manager) Queue(id string) []Queued { return append([]Queued{}, m.view(id).Queue...) }

// loadQueues starts the actor of every thread that kept a queue over a
// restart. The messages show up but do not run on their own: the user sends
// them with "steer" or sends something else.
func (m *Manager) loadQueues(ctx context.Context) {
	rows, err := m.db.QueryContext(ctx, `SELECT DISTINCT thread_id FROM thread_queue`)
	if err != nil {
		log.Printf("runtime: load queues: %v", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		m.runner(id)
	}
}

// startedTurn is a turn that began: what its recorder needs.
type startedTurn struct {
	thread  data.Thread
	session Session
	marker  Message
	change  modelChange
	usage   *ledger
	run     string
	root    string
	base    string
}

// start prepares and launches a turn; the actor then records it.
func (m *Manager) start(ctx context.Context, t data.Thread, q Queued) (startedTurn, error) {
	run, text, files := q.RunID, q.Text, q.Files
	p, ok := m.provider(t.Provider)
	if !ok {
		return startedTurn{}, ErrProvider.Msgf("%s", t.Provider)
	}
	var session, dir, sessionProvider, lastModel string
	err := m.db.QueryRowContext(ctx, `SELECT session_id, worktree_path, session_provider, last_turn_model FROM thread_runtime WHERE thread_id = ?`, t.ID).Scan(&session, &dir, &sessionProvider, &lastModel)
	if err != nil {
		return startedTurn{}, err
	}
	prompt := withAttachments(text, files)
	userPrompt := prompt
	var transferred bool
	if sessionProvider != t.Provider {
		session = ""
		history, err := m.Messages(ctx, t.ID)
		if err != nil {
			return startedTurn{}, err
		}
		prompt, transferred = portablePrompt(history, prompt)
		if transferred {
			// A provider file contains only its segment of this conversation.
			if _, err := m.db.ExecContext(ctx, `UPDATE thread_runtime SET portable_history = 1 WHERE thread_id = ?`, t.ID); err != nil {
				return startedTurn{}, err
			}
		}
	}
	startedAt := now()
	turn := Turn{ThreadID: t.ID, RunID: run, Dir: dir, Text: prompt, Model: t.Model, Effort: t.Effort, Session: session, Attachments: files,
		FullAccess: t.RuntimeMode != "approval-required"}
	historyChars := len(turn.Text) - len(userPrompt)
	if err := m.prepareMCP(ctx, t, &turn); err != nil {
		return startedTurn{}, err
	}
	if sessionProvider != t.Provider {
		// A previous visit to the destination may have left a live process.
		// Start a new conversation there with the current portable history.
		m.pool.CloseProcess(t.ID, t.Provider)
	}
	// The snapshot the turn's changes are diffed against when it ends.
	root := workspace.RepoRoot(ctx, dir)
	base := workspace.Snapshot(ctx, root)
	workspace.KeepSnapshot(ctx, root, t.ID, run, base)
	sessionRun, err := p.Start(ctx, turn)
	if err != nil {
		return startedTurn{}, err
	}
	var previous TurnModel
	_ = json.Unmarshal([]byte(lastModel), &previous)
	current := TurnModel{Provider: t.Provider, Model: &t.Model}
	modelChanged := (previous.Model != nil || (previous.ReportedModel != "" && t.Model != "")) &&
		(previous.Model == nil || *previous.Model != t.Model) && (previous.ReportedModel == "" || previous.ReportedModel != t.Model)
	providerChanged := previous.Provider != "" && previous.Provider != t.Provider
	var marker Message
	if providerChanged || modelChanged {
		marker = Message{ID: run + ":model-change", ThreadID: t.ID, RunID: run, Role: "model-change", CreatedAt: startedAt}
	} else if previous.Provider == t.Provider {
		current.ReportedModel = previous.ReportedModel
	}
	change := modelChange{From: previous, To: current, Handoff: transferred}
	m.setStatus(ctx, t.ID, "working", "")
	m.saveTurnModel(ctx, t.ID, current)
	if marker.ID != "" {
		m.saveModelChange(ctx, marker, change)
	}
	m.save(ctx, Message{ID: run + ":user", ThreadID: t.ID, RunID: run, Role: "user", Text: text, Detail: attachmentsDetail(files), CreatedAt: startedAt, SenderThreadID: q.Sender})
	return startedTurn{thread: t, session: sessionRun, marker: marker, change: change,
		usage: newLedger(userPrompt, len(turn.Text)-len(prompt), historyChars), run: run, root: root, base: base}, nil
}

type turnSettings struct {
	Provider   string
	Configured ReportedSettings
	Reported   ReportedSettings
}

func (m *Manager) ReportedSettings(ctx context.Context, t data.Thread) ReportedSettings {
	var raw string
	var settings turnSettings
	if m.db.QueryRowContext(ctx, `SELECT reported_settings FROM thread_runtime WHERE thread_id = ?`, t.ID).Scan(&raw) == nil &&
		json.Unmarshal([]byte(raw), &settings) == nil &&
		settings.Provider == t.Provider &&
		settings.Configured == (ReportedSettings{Model: t.Model, Effort: t.Effort}) {
		return settings.Reported
	}
	return ReportedSettings{}
}

func (m *Manager) setStatus(ctx context.Context, id, status, failure string) {
	if err := m.store.SetThreadState(ctx, id, status, failure); err != nil {
		log.Printf("runtime: status of %s: %v", id, err)
	} else if m.OnStateChange != nil {
		m.OnStateChange(id)
	}
	m.publish(Update{ThreadID: id, Status: status, Error: failure})
	m.emitJSON(ctx, id, EventStateChanged, map[string]string{"status": status, "error": failure})
}

// Message is stored with the thread it belongs to.
func (m *Manager) save(ctx context.Context, msg Message) {
	if m.persist(ctx, &msg, "", "") {
		m.publish(Update{ThreadID: msg.ThreadID, Message: &msg})
	}
}

// persist upserts a message in the read model. Together with it, in the same
// transaction, goes its event: typ, or the one the message calls for. A text
// that is still streaming records none until it is first seen or finished.
func (m *Manager) persist(ctx context.Context, msg *Message, typ, harnessID string) bool {
	streaming := 0
	if msg.Streaming {
		streaming = 1
	}
	redactSecret(msg)
	at := now()
	if msg.CreatedAt == "" {
		msg.CreatedAt = at
	}
	if typ == "" && !msg.Streaming {
		typ = messageEventType(*msg)
	}
	const upsert = `INSERT INTO thread_messages(id, thread_id, run_id, role, text, title, detail, status, streaming, created_at, updated_at, tool, sender_thread_id, code, params)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET text = excluded.text, title = excluded.title, detail = excluded.detail,
			status = excluded.status, streaming = excluded.streaming, updated_at = excluded.updated_at, tool = excluded.tool, code = excluded.code, params = excluded.params`
	args := []any{msg.ID, msg.ThreadID, msg.RunID, msg.Role, msg.Text, msg.Title, msg.Detail, msg.Status, streaming, msg.CreatedAt, at, toolJSON(msg.Tool), msg.SenderThreadID, msg.Code, paramsJSON(msg.Params)}
	if typ == "" {
		if _, err := m.db.ExecContext(ctx, upsert, args...); err != nil {
			log.Printf("runtime: save message: %v", err)
			return false
		}
		return true
	}
	payload, _ := json.Marshal(struct {
		Message
		HarnessID string `json:"harness_id,omitempty"`
	}{*msg, harnessID})
	m.evMu.Lock()
	defer m.evMu.Unlock()
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("runtime: save message: %v", err)
		return false
	}
	defer tx.Rollback()
	var e ThreadEvent
	if _, err = tx.ExecContext(ctx, upsert, args...); err == nil {
		e, err = appendEvent(ctx, tx, ThreadEvent{ThreadID: msg.ThreadID, RunID: msg.RunID, Type: typ, ItemID: msg.ID, Payload: payload})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		log.Printf("runtime: save message: %v", err)
		return false
	}
	m.sendEvent(e)
	return true
}

func paramsJSON(p map[string]any) string {
	if len(p) == 0 {
		return ""
	}
	b, _ := json.Marshal(p)
	return string(b)
}

func toolJSON(t *ToolCall) string {
	if t == nil {
		return ""
	}
	b, _ := json.Marshal(t)
	return string(b)
}

// Messages returns the conversation of a thread of this node, oldest first.
func (m *Manager) Messages(ctx context.Context, id string) ([]Message, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, thread_id, run_id, role, text, title, detail, status, streaming, created_at, tool, sender_thread_id, code, params
		FROM thread_messages WHERE thread_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var msg Message
		var streaming int
		var tool, params string
		if err = rows.Scan(&msg.ID, &msg.ThreadID, &msg.RunID, &msg.Role, &msg.Text, &msg.Title, &msg.Detail, &msg.Status, &streaming, &msg.CreatedAt, &tool, &msg.SenderThreadID, &msg.Code, &params); err != nil {
			return nil, err
		}
		if params != "" {
			_ = json.Unmarshal([]byte(params), &msg.Params)
		}
		if tool != "" {
			msg.Tool = &ToolCall{}
			if json.Unmarshal([]byte(tool), msg.Tool) != nil {
				msg.Tool = nil
			}
		}
		msg.Streaming = streaming == 1
		m.restoreAttachments(id, &msg)
		out = append(out, msg)
	}
	return out, rows.Err()
}

// Subscribe receives the updates of one thread, or of all with id "".
// Slow subscribers miss updates rather than block the runtime.
func (m *Manager) Subscribe(id string) (<-chan Update, func()) {
	ch := make(chan Update, 256)
	m.mu.Lock()
	m.subs[ch] = id
	m.mu.Unlock()
	return ch, func() {
		m.mu.Lock()
		delete(m.subs, ch)
		m.mu.Unlock()
	}
}

func (m *Manager) publish(u Update) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ch, id := range m.subs {
		if id != "" && id != u.ThreadID {
			continue
		}
		select {
		case ch <- u:
		default:
		}
	}
}

// Diff returns the changes of a thread's worktree against its base branch,
// including uncommitted work.
func (m *Manager) Diff(ctx context.Context, id string) (string, error) {
	d, err := m.DiffAgainst(ctx, id, "")
	return d.Diff, err
}

// WorkDiff is the diff of a thread's folder and what it was compared with.
type WorkDiff = workspace.WorkDiff

// DiffAgainst compares the thread's folder with against: a branch, from the
// point the work left it (merge-base), or "HEAD" for only what is not
// committed. Empty against uses the base branch the thread started from.
func (m *Manager) DiffAgainst(ctx context.Context, id, against string) (WorkDiff, error) {
	var dir, base string
	err := m.db.QueryRowContext(ctx, `SELECT worktree_path, base_branch FROM thread_runtime WHERE thread_id = ?`, id).Scan(&dir, &base)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkDiff{Branches: []string{}}, ErrNotFound
	}
	if err != nil {
		return WorkDiff{Branches: []string{}}, err
	}
	return workspace.Diff(ctx, dir, against, base)
}

// ErrDirtyWorktree says a worktree holds changes that are not committed.
var ErrDirtyWorktree = workspace.ErrDirtyWorktree

// worktreeDir returns the worktree dory created for a thread, or "" when the
// thread works in a checkout of the user or has no worktree.
func (m *Manager) worktreeDir(ctx context.Context, id string) (string, error) {
	dir, err := m.Dir(ctx, id)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(dir, filepath.Join(m.dataDir, "worktrees")) {
		return dir, nil
	}
	// A custom location Dory named after the thread.
	if workspace.IsLinkedWorktree(dir, id) {
		return dir, nil
	}
	return "", nil
}

// Dirty says whether dir has changes that are not committed.
func Dirty(ctx context.Context, dir string) (bool, error) { return workspace.Dirty(ctx, dir) }

// WorktreeState tells whether the worktree of a thread still exists and
// whether it has uncommitted changes. Shared is true while another thread
// that is not archived uses it.
func (m *Manager) WorktreeState(ctx context.Context, id string) (exists, dirty, shared bool, err error) {
	dir, err := m.worktreeDir(ctx, id)
	if err != nil || dir == "" {
		return false, false, false, err
	}
	if _, serr := os.Stat(dir); serr != nil {
		return false, false, false, nil
	}
	dirty, err = Dirty(ctx, dir)
	u, _ := m.WorktreeUse(ctx, dir, id)
	return true, dirty, u.Threads > 0, err
}

// KeptWorktree is the worktree of an archived thread that was not deleted.
type KeptWorktree struct {
	ThreadID string `json:"thread_id"`
	Path     string `json:"path"`
	Dirty    bool   `json:"dirty"`
}

// KeptWorktrees lists the worktrees of archived threads of this node that
// still exist and that no active thread uses.
func (m *Manager) KeptWorktrees(ctx context.Context) ([]KeptWorktree, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT t.id, r.worktree_path FROM threads t JOIN thread_runtime r ON r.thread_id = t.id
		WHERE t.archived_at IS NOT NULL AND t.node_id = ? AND r.worktree_path != ''`, m.node)
	if err != nil {
		return nil, err
	}
	var ids, paths []string
	for rows.Next() {
		var id, path string
		if err = rows.Scan(&id, &path); err != nil {
			rows.Close()
			return nil, err
		}
		ids, paths = append(ids, id), append(paths, path)
	}
	rows.Close()
	out := []KeptWorktree{}
	seen := map[string]bool{}
	for i, id := range ids {
		if seen[paths[i]] {
			continue
		}
		exists, dirty, shared, err := m.WorktreeState(ctx, id)
		if err != nil || !exists || shared {
			continue
		}
		seen[paths[i]] = true
		out = append(out, KeptWorktree{ThreadID: id, Path: paths[i], Dirty: dirty})
	}
	return out, nil
}

// Remove deletes the worktree of a thread when it is archived. A worktree
// that another thread still uses is kept. One with uncommitted changes is
// kept too and ErrDirtyWorktree is returned, unless force is set.
func (m *Manager) Remove(ctx context.Context, id string, force bool) error {
	dir, err := m.worktreeDir(ctx, id)
	if err != nil || dir == "" {
		return err
	}
	if users, _ := m.WorktreeUse(ctx, dir, id); users.Threads > 0 {
		// Another thread still works in this worktree.
		return nil
	}
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	return workspace.RemoveWorktree(ctx, dir, force)
}

// WorktreeUse tells how many threads that are not archived, apart from
// except, run in dir, how many of them are working, and the base branch the
// latest of them started from.
type WorktreeUse struct {
	Threads, Working int
	BaseBranch       string
}

func (m *Manager) WorktreeUse(ctx context.Context, dir, except string) (WorktreeUse, error) {
	var u WorktreeUse
	err := m.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(s.status = 'working'), 0),
		COALESCE((SELECT r2.base_branch FROM thread_runtime r2 JOIN threads t2 ON t2.id = r2.thread_id
			WHERE r2.worktree_path = ? AND r2.base_branch != '' ORDER BY t2.created_at DESC LIMIT 1), '')
		FROM thread_runtime r JOIN threads t ON t.id = r.thread_id
		LEFT JOIN thread_state s ON s.thread_id = t.id AND s.node_id = t.node_id
		WHERE r.worktree_path = ? AND t.archived_at IS NULL AND t.id != ?`, dir, dir, except).Scan(&u.Threads, &u.Working, &u.BaseBranch)
	return u, err
}

func newRunID() string {
	return "run-" + strings.ToLower(rand.Text()[:12]) + "-" + fmt.Sprint(time.Now().UnixNano()%1e6)
}

// syncBranch stores the branch the thread's folder is on when the agent
// switched it during the turn, so the thread and its task links follow it.
func (m *Manager) syncBranch(ctx context.Context, id string) {
	dir, err := m.Dir(ctx, id)
	if err != nil || dir == "" {
		return
	}
	branch, err := workspace.CurrentBranch(ctx, dir)
	if err != nil || branch == "" {
		return
	}
	t, err := m.store.Threads().Get(ctx, id)
	if err != nil || t.Branch == branch {
		return
	}
	if _, err := m.store.Threads().Patch(ctx, id, map[string]any{"branch": branch}); err != nil {
		log.Printf("runtime: branch of %s: %v", id, err)
	}
}

// Adopted returns the harness sessions that already are Dory threads.
func (m *Manager) Adopted(ctx context.Context) (map[string]string, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT session_id, thread_id FROM thread_runtime WHERE session_id <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var session, thread string
		if err = rows.Scan(&session, &thread); err != nil {
			return nil, err
		}
		out[session] = thread
	}
	return out, rows.Err()
}

// Follow keeps the threads of this node in step with the session files of
// their harness: what is written from the harness's own terminal is added to
// Dory as new events.
func (m *Manager) Follow(ctx context.Context, p Paths) {
	seen := map[string]time.Time{}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		rows, err := m.db.QueryContext(ctx, `SELECT r.thread_id, r.session_id, t.provider FROM thread_runtime r JOIN threads t ON t.id = r.thread_id
			WHERE r.session_id <> '' AND r.session_provider = t.provider AND r.portable_history = 0
			AND t.archived_at IS NULL`)
		if err != nil {
			continue
		}
		type link struct{ thread, session, provider string }
		var links []link
		for rows.Next() {
			var l link
			if rows.Scan(&l.thread, &l.session, &l.provider) == nil {
				links = append(links, l)
			}
		}
		rows.Close()
		for _, l := range links {
			if m.Running(l.thread) {
				continue
			}
			driver, ok := m.registry.Get(l.provider)
			if !ok || driver.External == nil {
				continue
			}
			path := driver.External.SessionFile(p, l.session)
			st, err := os.Stat(path)
			if path == "" || err != nil {
				continue
			}
			if prev, ok := seen[l.thread]; ok && prev.Equal(st.ModTime()) {
				continue
			}
			first := seen[l.thread].IsZero()
			seen[l.thread] = st.ModTime()
			// On start, only threads whose file changed after their last activity are rebuilt.
			if first {
				var last sql.NullString
				m.db.QueryRowContext(ctx, `SELECT MAX(updated_at) FROM thread_messages WHERE thread_id = ?`, l.thread).Scan(&last)
				if t, err := time.Parse(time.RFC3339Nano, last.String); err == nil && !st.ModTime().After(t) {
					continue
				}
			}
			if err := m.importExternal(ctx, l.thread, External{Provider: l.provider, ID: l.session, Path: path}); err != nil {
				log.Printf("runtime: sync %s: %v", l.thread, err)
			}
		}
	}
}

// importExternal adds to a thread what its harness file holds and the thread
// does not: only new items, as events; nothing is ever removed or rewritten.
func (m *Manager) importExternal(ctx context.Context, thread string, e External) error {
	msgs, err := m.ReadExternal(ctx, e)
	if err != nil || len(msgs) == 0 || m.Running(thread) {
		return err
	}
	var matches bool
	if err := m.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM thread_runtime r JOIN threads t ON t.id = r.thread_id
		WHERE r.thread_id = ? AND r.session_id = ? AND r.session_provider = ? AND t.provider = ? AND r.portable_history = 0
	)`, thread, e.ID, e.Provider, e.Provider).Scan(&matches); err != nil || !matches {
		return err
	}
	cutoff, err := m.importCutoff(ctx, thread)
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		if at, err := time.Parse(time.RFC3339Nano, msg.CreatedAt); err == nil && !at.After(cutoff) {
			continue
		}
		harnessID := msg.ID
		msg.ID = thread + ":" + harnessID
		var known bool
		if err := m.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM thread_messages WHERE id IN (?, ?))`, msg.ID, harnessID).Scan(&known); err != nil {
			return err
		}
		if known {
			continue
		}
		msg.ThreadID, msg.Streaming = thread, false
		m.restoreAttachments(thread, &msg)
		m.persist(ctx, &msg, EventItemCompleted, harnessID)
	}
	return nil
}

// importCutoff is the moment up to which a thread already holds what its
// harness file says. A live turn stores its items under its own ids, and a
// text under the time of its first fragment, while the harness writes it when
// the block ends: so everything up to the end of the last live turn, as well
// as up to the last message, is already there.
func (m *Manager) importCutoff(ctx context.Context, thread string) (time.Time, error) {
	var lastMessage, lastTurn sql.NullString
	if err := m.db.QueryRowContext(ctx, `SELECT MAX(created_at) FROM thread_messages WHERE thread_id = ?`, thread).Scan(&lastMessage); err != nil {
		return time.Time{}, err
	}
	if err := m.db.QueryRowContext(ctx, `SELECT MAX(created_at) FROM thread_events WHERE thread_id = ? AND type = ?`, thread, EventTurnEnded).Scan(&lastTurn); err != nil {
		return time.Time{}, err
	}
	var cutoff time.Time
	for _, s := range []string{lastMessage.String, lastTurn.String} {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil && t.After(cutoff) {
			cutoff = t
		}
	}
	return cutoff, nil
}

func (m *Manager) LatestVersion(id string) func(context.Context) string {
	d, ok := m.registry.Get(id)
	if !ok || d.Info().ID != d.Info().Driver || d.Versions == nil {
		return nil
	}
	return d.Versions.LatestVersion
}

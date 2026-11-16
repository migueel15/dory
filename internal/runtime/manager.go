package runtime

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/data"
)

var (
	ErrNotFound = errors.New("thread not found")
	ErrRemote   = errors.New("thread runs on another node")
	ErrProvider = errors.New("provider not available")
)

// Update is pushed to subscribers when a thread changes: a message upsert, a
// new status or a new queue.
type Update struct {
	ThreadID string   `json:"thread_id"`
	Message  *Message `json:"message,omitempty"`
	Status   string   `json:"status,omitempty"`
	Error    string   `json:"error,omitempty"`
	Queue    []Queued `json:"queue,omitempty"`
	// Reload asks clients to fetch the whole conversation again.
	Reload bool `json:"reload,omitempty"`
}

// Queued is a message waiting for the running turn to end.
type Queued struct {
	RunID string       `json:"run_id"`
	Text  string       `json:"text"`
	Files []Attachment `json:"files,omitempty"`
}

// Manager owns the threads of this node: it starts turns, keeps their
// messages and tells subscribers what changed.
type Manager struct {
	db        *sql.DB
	store     *data.Store
	node      string
	dataDir   string
	providers map[string]Provider
	// MCPURL and MCPToken are handed to every agent so it can reach Dory.
	MCPURL, MCPToken string

	mu      sync.Mutex
	running map[string]context.CancelFunc
	queues  map[string][]Queued
	subs    map[chan Update]string
	usage   map[string][]Window
	// asked marks threads whose agent asked the user something this turn.
	asked map[string]bool
	// steering marks threads whose turn was stopped to run the head of the queue now.
	steering map[string]bool
	// detected caches what Providers found.
	detected   []Status
	detectedAt time.Time
}

func NewManager(db *sql.DB, store *data.Store, node, dataDir string, providers ...Provider) *Manager {
	m := &Manager{db: db, store: store, node: node, dataDir: dataDir, providers: map[string]Provider{},
		running: map[string]context.CancelFunc{}, queues: map[string][]Queued{}, subs: map[chan Update]string{}, usage: map[string][]Window{}, asked: map[string]bool{}, steering: map[string]bool{}}
	for _, p := range providers {
		m.providers[p.ID()] = p
	}
	return m
}

// Recover marks as stopped the threads that were working when the node went
// down: their processes died with it.
func (m *Manager) Recover(ctx context.Context) error {
	threads, err := m.store.Threads().List(ctx, data.Filter{Fields: map[string]any{"node_id": m.node, "status": "working"}, Limit: 1000})
	if err != nil {
		return err
	}
	for _, t := range threads {
		t.Status = "error"
		if _, err = m.store.Threads().Put(ctx, t); err != nil {
			return err
		}
		m.save(ctx, Message{ID: newRunID(), ThreadID: t.ID, Role: "system", Text: "El nodo se reinició mientras el agente trabajaba; el turno se interrumpió.", CreatedAt: now()})
	}
	return nil
}

// Providers reports the harnesses of this machine, with their session. The
// answer is cached for a minute: detecting runs each harness.
func (m *Manager) Providers(ctx context.Context) []Status {
	m.mu.Lock()
	if m.detected != nil && time.Since(m.detectedAt) < time.Minute {
		out := m.detected
		m.mu.Unlock()
		return out
	}
	m.mu.Unlock()
	out := make([]Status, 0, len(m.providers))
	for _, id := range []string{"claude", "codex", "opencode", "replay"} {
		p, ok := m.providers[id]
		if !ok {
			continue
		}
		st := p.Detect(ctx)
		if a, ok := p.(Admin); ok && st.Installed {
			st.LoggedIn, st.Account = a.Session(ctx)
			st.Capabilities = append(st.Capabilities, "login", "update")
		}
		out = append(out, st)
	}
	m.mu.Lock()
	m.detected, m.detectedAt = out, time.Now()
	m.mu.Unlock()
	return out
}

// invalidate forgets the cached harness state after a login or update.
func (m *Manager) invalidate() {
	m.mu.Lock()
	m.detected = nil
	m.mu.Unlock()
}

// Usage returns the latest usage windows each provider reported.
func (m *Manager) Usage() map[string][]Window {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string][]Window, len(m.usage))
	for k, v := range m.usage {
		out[k] = v
	}
	return out
}

// NewThread describes a thread to create.
type NewThread struct {
	ProjectID, TaskID   *string
	Provider, Model     string
	Effort, RuntimeMode string
	Title               string
	// Worktree creates a branch and a worktree of the repository at RepoPath.
	Worktree           bool
	RepoPath           string
	Branch, BaseBranch string
}

// Create registers a thread on this node and prepares its working directory.
func (m *Manager) Create(ctx context.Context, n NewThread) (data.Thread, error) {
	if _, ok := m.providers[n.Provider]; !ok {
		return data.Thread{}, fmt.Errorf("%w: %s", ErrProvider, n.Provider)
	}
	t := data.Thread{NodeID: m.node, ProjectID: n.ProjectID, TaskID: n.TaskID, Provider: n.Provider, Model: n.Model,
		Effort: n.Effort, RuntimeMode: n.RuntimeMode, Title: n.Title, Branch: n.Branch}
	t, err := m.store.Threads().Put(ctx, t)
	if err != nil {
		return t, err
	}
	dir := n.RepoPath
	if n.Worktree {
		if n.RepoPath == "" || n.Branch == "" {
			return t, fmt.Errorf("%w: worktree needs a repository path and a branch", data.ErrInvalid)
		}
		dir = filepath.Join(m.dataDir, "worktrees", t.ID)
		args := []string{"-C", n.RepoPath, "worktree", "add"}
		if exists(ctx, n.RepoPath, n.Branch) {
			args = append(args, dir, n.Branch)
		} else {
			args = append(args, "-b", n.Branch, dir)
			if n.BaseBranch != "" {
				args = append(args, n.BaseBranch)
			}
		}
		if out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
			return t, fmt.Errorf("git worktree: %s", strings.TrimSpace(string(out)))
		}
	}
	if dir == "" {
		dir = filepath.Join(m.dataDir, "scratch", t.ID)
		if err = os.MkdirAll(dir, 0o755); err != nil {
			return t, err
		}
	}
	_, err = m.db.ExecContext(ctx, `INSERT INTO thread_runtime(thread_id, worktree_path, base_branch) VALUES (?, ?, ?)`, t.ID, dir, n.BaseBranch)
	return t, err
}

func exists(ctx context.Context, repo, branch string) bool {
	return exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
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
	t, err := m.local(ctx, id)
	if err != nil {
		return err
	}
	run := newRunID()
	m.mu.Lock()
	if _, busy := m.running[id]; busy {
		m.queues[id] = append(m.queues[id], Queued{RunID: run, Text: text, Files: files})
		q := append([]Queued(nil), m.queues[id]...)
		m.mu.Unlock()
		m.publish(Update{ThreadID: id, Queue: q})
		return nil
	}
	turnCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.running[id] = cancel
	m.mu.Unlock()
	if err = m.start(turnCtx, t, run, text, files); err != nil {
		cancel()
		m.mu.Lock()
		delete(m.running, id)
		m.mu.Unlock()
		return err
	}
	return nil
}

// Ask records a question of the agent and marks the thread as waiting for
// the user; the answer arrives as the next message.
func (m *Manager) Ask(ctx context.Context, id, question string) error {
	if _, err := m.local(ctx, id); err != nil {
		return err
	}
	m.save(ctx, Message{ID: newRunID(), ThreadID: id, Role: "system", Title: "Pregunta", Text: question, Status: "waiting", CreatedAt: now()})
	m.mu.Lock()
	m.asked[id] = true
	m.mu.Unlock()
	return nil
}

// Interrupt stops the running turn and drops the queue.
func (m *Manager) Interrupt(ctx context.Context, id string) error {
	if _, err := m.local(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	cancel := m.running[id]
	delete(m.queues, id)
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.publish(Update{ThreadID: id, Queue: []Queued{}})
	return nil
}

// Running reports whether a turn of the thread is in progress here.
func (m *Manager) Running(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.running[id]
	return ok
}

// Queue returns the messages waiting in a thread.
func (m *Manager) Queue(id string) []Queued {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Queued{}, m.queues[id]...)
}

func (m *Manager) start(ctx context.Context, t data.Thread, run, text string, files []Attachment) error {
	p, ok := m.providers[t.Provider]
	if !ok {
		return fmt.Errorf("%w: %s", ErrProvider, t.Provider)
	}
	var session, dir string
	err := m.db.QueryRowContext(ctx, `SELECT session_id, worktree_path FROM thread_runtime WHERE thread_id = ?`, t.ID).Scan(&session, &dir)
	if err != nil {
		return err
	}
	m.save(ctx, Message{ID: run + ":user", ThreadID: t.ID, RunID: run, Role: "user", Text: text, Detail: attachmentsDetail(files), CreatedAt: now()})
	events, err := p.Run(ctx, Turn{RunID: run, Dir: dir, Text: withAttachments(text, files), Model: t.Model, Effort: t.Effort, Session: session,
		FullAccess: t.RuntimeMode != "approval-required", MCPURL: m.mcpURL(t.ID), MCPToken: m.MCPToken})
	if err != nil {
		return err
	}
	m.setStatus(ctx, t.ID, "working", "")
	go m.follow(ctx, t, events)
	return nil
}

// follow stores what a turn emits until it ends, then starts the next
// queued message, if any.
func (m *Manager) follow(ctx context.Context, t data.Thread, events <-chan Event) {
	bg := context.WithoutCancel(ctx)
	status, failure := "idle", ""
	for e := range events {
		switch {
		case e.Message != nil:
			msg := *e.Message
			msg.ThreadID = t.ID
			m.save(bg, msg)
		case e.Session != "":
			if _, err := m.db.ExecContext(bg, `UPDATE thread_runtime SET session_id = ? WHERE thread_id = ?`, e.Session, t.ID); err != nil {
				log.Printf("runtime: save session: %v", err)
			}
		case e.Usage != nil:
			m.mu.Lock()
			m.usage[t.Provider] = e.Usage
			m.mu.Unlock()
		case e.Done:
			if e.Err != "" {
				status, failure = "error", e.Err
				if ctx.Err() != nil {
					status, failure = "idle", ""
					m.save(bg, Message{ID: newRunID(), ThreadID: t.ID, Role: "interrupt", Text: "Turno interrumpido", CreatedAt: now()})
				} else {
					m.save(bg, Message{ID: newRunID(), ThreadID: t.ID, Role: "system", Text: e.Err, Status: "failed", CreatedAt: now()})
				}
			}
		}
	}
	m.mu.Lock()
	if m.asked[t.ID] && status == "idle" {
		status = "waiting"
	}
	delete(m.asked, t.ID)
	next := m.queues[t.ID]
	var q Queued
	steered := m.steering[t.ID]
	delete(m.steering, t.ID)
	if len(next) > 0 && (ctx.Err() == nil || steered) {
		q, m.queues[t.ID] = next[0], next[1:]
	}
	if steered && q.RunID != "" {
		// The stopped turn's context is gone: the steered one gets its own.
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(bg)
		m.running[t.ID] = cancel
	}
	if q.RunID == "" {
		delete(m.running, t.ID)
	}
	remaining := append([]Queued{}, m.queues[t.ID]...)
	m.mu.Unlock()
	m.setStatus(bg, t.ID, status, failure)
	if q.RunID != "" {
		m.publish(Update{ThreadID: t.ID, Queue: remaining})
		fresh, err := m.store.Threads().Get(bg, t.ID)
		if err == nil {
			err = m.start(ctx, fresh, q.RunID, q.Text, q.Files)
		}
		if err != nil {
			m.mu.Lock()
			delete(m.running, t.ID)
			m.mu.Unlock()
			m.setStatus(bg, t.ID, "error", err.Error())
		}
	}
}

func (m *Manager) setStatus(ctx context.Context, id, status, failure string) {
	t, err := m.store.Threads().Get(ctx, id)
	if err != nil {
		log.Printf("runtime: status of %s: %v", id, err)
		return
	}
	at := now()
	t.Status, t.LastActivityAt = status, &at
	if status != "working" {
		// A finished turn is unread until the user opens the thread.
		t.ReadAt = nil
	}
	if _, err = m.store.Threads().Put(ctx, t); err != nil {
		log.Printf("runtime: status of %s: %v", id, err)
	}
	m.publish(Update{ThreadID: id, Status: status, Error: failure})
}

// Message is stored with the thread it belongs to.
func (m *Manager) save(ctx context.Context, msg Message) {
	streaming := 0
	if msg.Streaming {
		streaming = 1
	}
	at := now()
	if msg.CreatedAt == "" {
		msg.CreatedAt = at
	}
	_, err := m.db.ExecContext(ctx, `INSERT INTO thread_messages(id, thread_id, run_id, role, text, title, detail, status, streaming, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET text = excluded.text, title = excluded.title, detail = excluded.detail,
			status = excluded.status, streaming = excluded.streaming, updated_at = excluded.updated_at`,
		msg.ID, msg.ThreadID, msg.RunID, msg.Role, msg.Text, msg.Title, msg.Detail, msg.Status, streaming, msg.CreatedAt, at)
	if err != nil {
		log.Printf("runtime: save message: %v", err)
		return
	}
	m.publish(Update{ThreadID: msg.ThreadID, Message: &msg})
}

// Messages returns the conversation of a thread of this node, oldest first.
func (m *Manager) Messages(ctx context.Context, id string) ([]Message, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, thread_id, run_id, role, text, title, detail, status, streaming, created_at
		FROM thread_messages WHERE thread_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var msg Message
		var streaming int
		if err = rows.Scan(&msg.ID, &msg.ThreadID, &msg.RunID, &msg.Role, &msg.Text, &msg.Title, &msg.Detail, &msg.Status, &streaming, &msg.CreatedAt); err != nil {
			return nil, err
		}
		msg.Streaming = streaming == 1
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
	var dir, base string
	err := m.db.QueryRowContext(ctx, `SELECT worktree_path, base_branch FROM thread_runtime WHERE thread_id = ?`, id).Scan(&dir, &base)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--git-dir").Run() != nil {
		return "", nil
	}
	ref := "HEAD"
	if base != "" {
		if out, err := exec.CommandContext(ctx, "git", "-C", dir, "merge-base", base, "HEAD").Output(); err == nil {
			ref = strings.TrimSpace(string(out))
		}
	}
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "diff", ref).Output()
	return string(out), err
}

// Remove deletes the worktree of a thread when it is archived.
func (m *Manager) Remove(ctx context.Context, id string) error {
	dir, err := m.Dir(ctx, id)
	if err != nil || !strings.HasPrefix(dir, filepath.Join(m.dataDir, "worktrees")) {
		return err
	}
	_ = m.Interrupt(ctx, id)
	return exec.CommandContext(ctx, "git", "-C", dir, "worktree", "remove", "--force", dir).Run()
}

func newRunID() string {
	return "run-" + strings.ToLower(rand.Text()[:12]) + "-" + fmt.Sprint(time.Now().UnixNano()%1e6)
}

func gitCmd(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).CombinedOutput()
}

func (m *Manager) mcpURL(thread string) string {
	if m.MCPURL == "" {
		return ""
	}
	return m.MCPURL + "?thread=" + thread
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

// Adopt turns a harness session into a Dory thread: its history is copied
// and the next message resumes the same session in its folder.
func (m *Manager) Adopt(ctx context.Context, e External, projectID *string) (data.Thread, error) {
	if adopted, err := m.Adopted(ctx); err == nil && adopted[e.ID] != "" {
		return m.store.Threads().Get(ctx, adopted[e.ID])
	}
	if _, ok := m.providers[e.Provider]; !ok {
		return data.Thread{}, fmt.Errorf("%w: %s", ErrProvider, e.Provider)
	}
	msgs, err := ExternalMessages(e)
	if err != nil {
		return data.Thread{}, err
	}
	t, err := m.store.Threads().Put(ctx, data.Thread{NodeID: m.node, ProjectID: projectID, Provider: e.Provider, Title: e.Title, Branch: e.Branch})
	if err != nil {
		return t, err
	}
	if _, err = m.db.ExecContext(ctx, `INSERT INTO thread_runtime(thread_id, session_id, worktree_path) VALUES (?, ?, ?)`, t.ID, e.ID, e.Dir); err != nil {
		return t, err
	}
	for _, msg := range msgs {
		msg.ThreadID = t.ID
		msg.ID = t.ID + ":" + msg.ID
		m.save(ctx, msg)
	}
	return t, nil
}

// Follow keeps the threads of this node in step with the session files of
// their harness: what is written from the harness's own terminal shows up in
// Dory, and after each turn the history is the one the harness saved.
func (m *Manager) Follow(ctx context.Context, home string) {
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
			WHERE r.session_id <> '' AND t.archived_at IS NULL AND t.provider IN ('claude', 'codex')`)
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
			path := sessionFile(home, l.provider, l.session)
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
			if err := m.resync(ctx, l.thread, External{Provider: l.provider, ID: l.session, path: path}); err != nil {
				log.Printf("runtime: sync %s: %v", l.thread, err)
			}
		}
	}
}

// resync replaces the conversation of a thread with the one in its file.
func (m *Manager) resync(ctx context.Context, thread string, e External) error {
	msgs, err := ExternalMessages(e)
	if err != nil || len(msgs) == 0 {
		return err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM thread_messages WHERE thread_id = ?`, thread); err != nil {
		return err
	}
	at := now()
	for _, msg := range msgs {
		if msg.CreatedAt == "" {
			msg.CreatedAt = at
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO thread_messages(id, thread_id, run_id, role, text, title, detail, status, streaming, created_at, updated_at)
			VALUES (?, ?, '', ?, ?, ?, ?, ?, 0, ?, ?) ON CONFLICT(id) DO NOTHING`,
			thread+":"+msg.ID, thread, msg.Role, msg.Text, msg.Title, msg.Detail, msg.Status, msg.CreatedAt, at); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	m.publish(Update{ThreadID: thread, Reload: true})
	return nil
}

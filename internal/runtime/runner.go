package runtime

import (
	"context"
	"encoding/json"
	"log"
	"slices"
	"sync/atomic"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/harness"
)

// threadRunner is the actor of one thread: a goroutine that owns everything
// that changes while the thread works (the open turn, its queue, the pending
// requests and settings). Callers never touch that state; they send a command
// and wait for its answer, so the life cycle of a turn has no races.
type threadRunner struct {
	m    *Manager
	id   string
	cmds chan *command
	done chan struct{}

	// view is the last state the actor published, for readers that must not
	// wait for the actor (Running, Queue, Pending, Active).
	view atomic.Pointer[runnerView]

	// Everything below belongs to the actor goroutine.
	state  runnerState
	cancel context.CancelFunc
	// runID names the open turn, so a late report of an old one is ignored.
	runID   string
	session Session
	queue   []Queued
	// steering marks a turn stopped to run the head of the queue now.
	steering bool
	pending  *ThreadSettings
	// waiters hold the ask_user calls waiting for the user, by request id.
	waiters map[string]chan [][]string
	// secrets hold the request_secret calls waiting for the user.
	secrets map[string]secretWaiter
	// native are the requests the harness of the open turn made.
	native map[string]bool
}

type runnerState int

const (
	// stateIdle: no turn. A queue loaded after a restart waits here.
	stateIdle runnerState = iota
	// stateStarting: the turn is being prepared (only seen inside the actor).
	stateStarting
	// stateRunning: a turn is in progress.
	stateRunning
	// stateWaitingInput: the turn waits for the user to answer a question,
	// a permission or a secret.
	stateWaitingInput
	// stateStopping: the turn was cancelled and its end is pending.
	stateStopping
)

func (s runnerState) String() string {
	return [...]string{"idle", "starting", "running", "waiting_input", "stopping"}[s]
}

// busy says a turn exists, whatever its phase.
func (s runnerState) busy() bool { return s != stateIdle }

// runnerView is what other goroutines may read about a runner.
type runnerView struct {
	State   runnerState
	Queue   []Queued
	Pending *ThreadSettings
}

type cmdKind int

const (
	cmdSend cmdKind = iota
	cmdEditQueued
	cmdCancelQueued
	cmdSteer
	cmdInterrupt
	cmdAnswer
	cmdAnswerSecret
	cmdConfigure
	cmdClear
	cmdAsk
	cmdForgetAsk
	cmdAskSecret
	cmdForgetSecret
	cmdRequestOpened
	cmdTurnEnded
)

// command is one order for the actor. Fields are used by the kinds that need
// them; the answer comes back on reply.
type command struct {
	kind cmdKind
	ctx  context.Context

	queued Queued
	run    string
	text   string

	req     string
	answers [][]string
	values  map[string]string
	decline bool
	prompt  SecretPrompt

	settings ThreadSettings
	// force is Configure from the user, which applies at once except a
	// provider switch; an agent's change waits for a running turn.
	deferred bool

	thread  data.Thread
	outcome turnOutcome

	reply chan result
}

// result is the answer to a command.
type result struct {
	err    error
	thread data.Thread
	// applied says when a configuration took effect.
	applied string
	// answers and secret are the channels an ask hands back.
	req     string
	answers chan [][]string
	secret  chan secretReply
}

type handler func(r *threadRunner, c *command) result

type transition struct {
	state runnerState
	kind  cmdKind
}

// transitions is the table of the state machine: what each command does in
// each state. A pair that is not listed is refused with an error and leaves
// the state as it was.
var transitions = map[transition]handler{}

func on(kind cmdKind, h handler, states ...runnerState) {
	for _, s := range states {
		transitions[transition{s, kind}] = h
	}
}

var (
	anyState    = []runnerState{stateIdle, stateStarting, stateRunning, stateWaitingInput, stateStopping}
	turnStates  = []runnerState{stateRunning, stateWaitingInput}
	busyStates  = []runnerState{stateStarting, stateRunning, stateWaitingInput, stateStopping}
	errNotQueue = ErrNotFound
)

func init() {
	// A message starts a turn when idle and waits in the queue otherwise.
	on(cmdSend, (*threadRunner).startNow, stateIdle)
	on(cmdSend, (*threadRunner).enqueue, busyStates...)
	on(cmdEditQueued, (*threadRunner).editQueued, anyState...)
	on(cmdCancelQueued, (*threadRunner).cancelQueued, anyState...)
	// Steering stops the turn to run the chosen message; when idle (a queue
	// kept over a restart) it just runs it.
	on(cmdSteer, (*threadRunner).steerIdle, stateIdle)
	on(cmdSteer, (*threadRunner).steer, turnStates...)
	on(cmdSteer, (*threadRunner).steerStopping, stateStopping)
	on(cmdInterrupt, (*threadRunner).interrupt, anyState...)
	on(cmdAnswer, (*threadRunner).answer, anyState...)
	on(cmdAnswerSecret, (*threadRunner).answerSecret, anyState...)
	on(cmdConfigure, (*threadRunner).configureNow, stateIdle)
	on(cmdConfigure, (*threadRunner).configureBusy, busyStates...)
	on(cmdClear, (*threadRunner).clear, stateIdle)
	on(cmdAsk, (*threadRunner).ask, anyState...)
	on(cmdForgetAsk, (*threadRunner).forgetAsk, anyState...)
	on(cmdAskSecret, (*threadRunner).askSecret, anyState...)
	on(cmdForgetSecret, (*threadRunner).forgetSecret, anyState...)
	on(cmdRequestOpened, (*threadRunner).requestOpened, turnStates...)
	on(cmdTurnEnded, (*threadRunner).turnEnded, turnStates...)
	on(cmdTurnEnded, (*threadRunner).turnEnded, stateStopping)
}

var (
	// ErrThreadBusy is a command that needs the thread to be idle.
	ErrThreadBusy = ErrBusy
	// ErrRequestNotPending is an answer to a request that is gone.
	ErrRequestNotPending = ErrNoRequest
	errBadState          = apperr.New(apperr.Busy, "the thread cannot do that in its current state")
)

// refused is the error of a command that has no transition in the state.
func refused(kind cmdKind) error {
	switch kind {
	case cmdClear:
		return ErrThreadBusy
	case cmdAnswer, cmdAnswerSecret, cmdRequestOpened:
		return ErrRequestNotPending
	}
	return errBadState
}

func (m *Manager) newRunner(id string) *threadRunner {
	r := &threadRunner{m: m, id: id, cmds: make(chan *command), done: make(chan struct{}),
		waiters: map[string]chan [][]string{}, secrets: map[string]secretWaiter{}, native: map[string]bool{}}
	r.queue = m.loadQueue(id)
	r.publishView()
	return r
}

// runner returns the actor of a thread, creating it when needed.
func (m *Manager) runner(id string) *threadRunner {
	m.runnersMu.Lock()
	defer m.runnersMu.Unlock()
	r := m.runners[id]
	if r == nil {
		r = m.newRunner(id)
		m.runners[id] = r
		go r.loop()
	}
	return r
}

// lookup returns the actor of a thread only if it exists.
func (m *Manager) lookup(id string) *threadRunner {
	m.runnersMu.Lock()
	defer m.runnersMu.Unlock()
	return m.runners[id]
}

// do sends a command to the actor of a thread and waits for its answer. An
// actor that ended for being idle is replaced by a new one.
func (m *Manager) do(id string, c *command) result {
	if c.reply == nil {
		c.reply = make(chan result, 1)
	}
	for {
		r := m.runner(id)
		select {
		case r.cmds <- c:
			return <-c.reply
		case <-r.done:
		}
	}
}

func (r *threadRunner) publishView() {
	v := &runnerView{State: r.state, Queue: slices.Clone(r.queue)}
	if r.pending != nil {
		p := *r.pending
		v.Pending = &p
	}
	r.view.Store(v)
}

func (r *threadRunner) setState(s runnerState) {
	r.state = s
	r.publishView()
}

// waiting is true while something the user must answer is open.
func (r *threadRunner) waiting() bool {
	return len(r.waiters)+len(r.secrets)+len(r.native) > 0
}

// settle moves between Running and WaitingInput as requests open and close.
func (r *threadRunner) settle() {
	switch {
	case r.state == stateRunning && r.waiting():
		r.setState(stateWaitingInput)
	case r.state == stateWaitingInput && !r.waiting():
		r.setState(stateRunning)
	}
}

func (r *threadRunner) loop() {
	defer close(r.done)
	timer := time.NewTimer(r.m.runnerIdle)
	defer timer.Stop()
	for {
		select {
		case c := <-r.cmds:
			c.reply <- r.handle(c)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(r.m.runnerIdle)
		case <-timer.C:
			if r.state == stateIdle && len(r.queue) == 0 && !r.waiting() {
				// Leave the registry first: whoever already holds this
				// runner sees done closed and asks for a new one.
				r.m.runnersMu.Lock()
				if r.m.runners[r.id] == r {
					delete(r.m.runners, r.id)
				}
				r.m.runnersMu.Unlock()
				return
			}
			timer.Reset(r.m.runnerIdle)
		}
	}
}

func (r *threadRunner) handle(c *command) result {
	h, ok := transitions[transition{r.state, c.kind}]
	if !ok {
		return result{err: refused(c.kind)}
	}
	return h(r, c)
}

// ---- queue

// persist stores the queue so a restart keeps it.
func (r *threadRunner) persist() {
	r.publishView()
	if err := r.m.saveQueue(r.id, r.queue); err != nil {
		log.Printf("runtime: save queue of %s: %v", r.id, err)
	}
	r.m.publish(Update{ThreadID: r.id, Queue: slices.Clone(r.queue)})
}

func (m *Manager) loadQueue(id string) []Queued {
	rows, err := m.db.Query(`SELECT run_id, text, files, sender_thread_id FROM thread_queue WHERE thread_id = ? ORDER BY position`, id)
	if err != nil {
		log.Printf("runtime: load queue of %s: %v", id, err)
		return nil
	}
	defer rows.Close()
	var out []Queued
	for rows.Next() {
		var q Queued
		var files string
		if rows.Scan(&q.RunID, &q.Text, &files, &q.Sender) == nil {
			_ = json.Unmarshal([]byte(files), &q.Files)
			out = append(out, q)
		}
	}
	return out
}

func (m *Manager) saveQueue(id string, queue []Queued) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM thread_queue WHERE thread_id = ?`, id); err != nil {
		return err
	}
	at := now()
	for i, q := range queue {
		files, _ := json.Marshal(q.Files)
		if _, err = tx.Exec(`INSERT INTO thread_queue(thread_id, run_id, position, text, files, sender_thread_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, q.RunID, i, q.Text, string(files), q.Sender, at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *threadRunner) enqueue(c *command) result {
	r.queue = append(r.queue, c.queued)
	r.persist()
	return result{}
}

func (r *threadRunner) index(run string) int {
	return slices.IndexFunc(r.queue, func(q Queued) bool { return q.RunID == run })
}

func (r *threadRunner) editQueued(c *command) result {
	i := r.index(c.run)
	if i < 0 {
		return result{err: errNotQueue}
	}
	r.queue[i].Text = c.text
	r.persist()
	return result{}
}

func (r *threadRunner) cancelQueued(c *command) result {
	i := r.index(c.run)
	if i < 0 {
		return result{err: errNotQueue}
	}
	r.queue = slices.Delete(r.queue, i, i+1)
	r.persist()
	return result{}
}

func (r *threadRunner) toHead(run string) error {
	i := r.index(run)
	if i < 0 {
		return errNotQueue
	}
	head := r.queue[i]
	r.queue = append([]Queued{head}, slices.Delete(r.queue, i, i+1)...)
	r.persist()
	return nil
}

func (r *threadRunner) steer(c *command) result {
	if err := r.toHead(c.run); err != nil {
		return result{err: err}
	}
	r.steering = true
	r.stop()
	return result{}
}

func (r *threadRunner) steerStopping(c *command) result {
	if err := r.toHead(c.run); err != nil {
		return result{err: err}
	}
	r.steering = true
	return result{}
}

func (r *threadRunner) steerIdle(c *command) result {
	if err := r.toHead(c.run); err != nil {
		return result{err: err}
	}
	next := r.queue[0]
	r.queue = r.queue[1:]
	r.persist()
	return result{err: r.begin(c.ctx, next)}
}

// stop cancels the open turn; its end arrives as cmdTurnEnded.
func (r *threadRunner) stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.setState(stateStopping)
}

func (r *threadRunner) interrupt(c *command) result {
	r.queue = nil
	r.persist()
	if r.state.busy() {
		r.stop()
	}
	return result{}
}

// ---- turns

func (r *threadRunner) startNow(c *command) result {
	return result{err: r.begin(c.ctx, c.queued)}
}

// begin starts a turn. The turn outlives the caller's context.
func (r *threadRunner) begin(ctx context.Context, q Queued) error {
	t, err := r.m.local(ctx, r.id)
	if err != nil {
		return err
	}
	r.setState(stateStarting)
	turnCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	run, err := r.m.start(turnCtx, t, q)
	if err != nil {
		cancel()
		r.setState(stateIdle)
		return err
	}
	r.cancel, r.session, r.runID = cancel, run.session, run.run
	r.setState(stateRunning)
	go r.follow(turnCtx, run)
	return nil
}

// follow records the turn and then tells the actor it ended.
func (r *threadRunner) follow(ctx context.Context, run startedTurn) {
	out := r.m.record(ctx, run, func(req harness.Request) {
		// Asynchronous: the actor may be answering through this very turn.
		go r.m.do(r.id, &command{kind: cmdRequestOpened, req: req.ID, run: run.run})
	})
	out.interrupted = ctx.Err() != nil
	r.m.do(r.id, &command{kind: cmdTurnEnded, thread: run.thread, outcome: out})
}

func (r *threadRunner) requestOpened(c *command) result {
	if c.run == r.runID {
		r.native[c.req] = true
		r.settle()
	}
	return result{}
}

// turnEnded closes the turn: settings an agent chose apply, the thread
// state is stored and the next queued message starts.
func (r *threadRunner) turnEnded(c *command) result {
	m, t, out := r.m, c.thread, c.outcome
	bg := context.Background()
	r.session, r.cancel = nil, nil
	// The process is gone: nothing can answer its requests any more.
	clear(r.native)
	steered := r.steering
	r.steering = false
	var next *Queued
	if len(r.queue) > 0 && (!out.interrupted || steered) {
		q := r.queue[0]
		next = &q
		r.queue = r.queue[1:]
		r.persist()
	}
	var configured error
	if r.pending != nil {
		settings := *r.pending
		r.pending = nil
		_, configured = m.applySettings(bg, r.id, settings, false)
	}
	r.setState(stateIdle)
	m.setStatus(bg, r.id, out.status, out.failure)
	if configured != nil {
		m.save(bg, Message{ID: newRunID(), ThreadID: r.id, Role: "system", Text: "No se pudo cambiar el modelo: " + configured.Error(), Status: "failed", CreatedAt: now()})
	}
	if next == nil && t.ParentID != nil {
		m.notifyParent(bg, t, out.status, out.interrupted)
	}
	if next != nil {
		if err := r.begin(bg, *next); err != nil {
			m.setStatus(bg, r.id, "error", err.Error())
		}
	}
	return result{}
}

// ---- requests

func (r *threadRunner) ask(c *command) result {
	req := newRunID()
	ch := make(chan [][]string, 1)
	r.waiters[req] = ch
	r.settle()
	return result{req: req, answers: ch}
}

func (r *threadRunner) forgetAsk(c *command) result {
	delete(r.waiters, c.req)
	r.settle()
	return result{}
}

func (r *threadRunner) askSecret(c *command) result {
	req := newRunID()
	ch := make(chan secretReply, 1)
	r.secrets[req] = secretWaiter{prompt: c.prompt, reply: ch}
	r.settle()
	return result{req: req, secret: ch}
}

func (r *threadRunner) forgetSecret(c *command) result {
	delete(r.secrets, c.req)
	r.settle()
	return result{}
}

func (r *threadRunner) answer(c *command) result {
	if w, ok := r.waiters[c.req]; ok {
		// An ask_user call: its tool call returns the answers.
		delete(r.waiters, c.req)
		w <- c.answers
		r.settle()
		return result{err: r.m.resolved(c.ctx, r.id, c.req)}
	}
	var exists bool
	if err := r.m.db.QueryRowContext(c.ctx, `SELECT EXISTS(SELECT 1 FROM thread_requests WHERE id = ? AND thread_id = ? AND kind <> 'secret')`, c.req, r.id).Scan(&exists); err != nil {
		return result{err: err}
	}
	if r.session == nil || !exists {
		return result{err: ErrRequestNotPending}
	}
	if err := r.session.Respond(c.req, c.answers); err != nil {
		return result{err: err}
	}
	delete(r.native, c.req)
	r.settle()
	return result{err: r.m.resolved(c.ctx, r.id, c.req)}
}

func (r *threadRunner) answerSecret(c *command) result {
	w, ok := r.secrets[c.req]
	if !ok {
		return result{err: ErrRequestNotPending}
	}
	if !c.decline {
		names := secretNames(w.prompt)
		if len(c.values) != len(names) {
			return result{err: ErrSecretFields}
		}
		for _, n := range names {
			if _, ok := c.values[n]; !ok {
				return result{err: ErrSecretFields}
			}
		}
	}
	delete(r.secrets, c.req)
	w.reply <- secretReply{values: c.values, decline: c.decline}
	r.settle()
	return result{err: r.m.resolved(c.ctx, r.id, c.req)}
}

// ---- settings and clear

func (r *threadRunner) configureNow(c *command) result {
	t, err := r.m.applySettings(c.ctx, r.id, c.settings, false)
	return result{err: err, thread: t, applied: AppliedNow}
}

func (r *threadRunner) configureBusy(c *command) result {
	if c.deferred {
		merged := mergeSettings(derefSettings(r.pending), c.settings)
		r.pending = &merged
		r.publishView()
		r.m.publish(Update{ThreadID: r.id, Reload: true})
		t, _ := r.m.local(c.ctx, r.id)
		return result{thread: t, applied: AppliedAfterTurn}
	}
	t, err := r.m.applySettings(c.ctx, r.id, c.settings, true)
	return result{err: err, thread: t, applied: AppliedNow}
}

func derefSettings(s *ThreadSettings) ThreadSettings {
	if s == nil {
		return ThreadSettings{}
	}
	return *s
}

func (r *threadRunner) clear(c *command) result {
	return result{err: r.m.clearSession(c.ctx, r.id)}
}

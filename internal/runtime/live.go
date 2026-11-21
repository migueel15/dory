package runtime

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// liveProc is a harness process that outlives its turn: the next turn of the
// same thread reuses it, so what the agent started in the background (a dev
// server, a database) keeps running between turns, as in an interactive CLI.
type liveProc interface {
	// Busy is true while a turn runs on the process.
	Busy() bool
	// IdleSince is when its last turn ended.
	IdleSince() time.Time
	// Close ends the process and what it started.
	Close()
}

// liveIdle is how long an idle process stays open when its thread has no
// listening services; a thread with services keeps its process.
const liveIdle = 30 * time.Minute

var live = struct {
	sync.Mutex
	procs map[string]liveProc
}{procs: map[string]liveProc{}}

func liveKey(thread, provider string) string { return thread + "\x00" + provider }

func liveGet(thread, provider string) liveProc {
	live.Lock()
	defer live.Unlock()
	return live.procs[liveKey(thread, provider)]
}

func liveSet(thread, provider string, p liveProc) {
	live.Lock()
	live.procs[liveKey(thread, provider)] = p
	live.Unlock()
}

// liveDrop forgets p if it still is the process of the thread.
func liveDrop(thread, provider string, p liveProc) {
	live.Lock()
	if live.procs[liveKey(thread, provider)] == p {
		delete(live.procs, liveKey(thread, provider))
	}
	live.Unlock()
}

// take removes and returns the processes whose key matches.
func take(match func(key string) bool) []liveProc {
	live.Lock()
	defer live.Unlock()
	var out []liveProc
	for k, p := range live.procs {
		if match(k) {
			out = append(out, p)
			delete(live.procs, k)
		}
	}
	return out
}

func closeAll(ps []liveProc) {
	var wg sync.WaitGroup
	for _, p := range ps {
		wg.Add(1)
		go func() { defer wg.Done(); p.Close() }()
	}
	wg.Wait()
}

// CloseLive ends the harness processes of a thread, such as when it is archived.
func CloseLive(thread string) {
	closeAll(take(func(k string) bool { return strings.HasPrefix(k, thread+"\x00") }))
}

// CloseAllLive ends every harness process; Dory calls it when it stops.
func CloseAllLive() { closeAll(take(func(string) bool { return true })) }

var reapOnce sync.Once

// reapLive closes the processes idle for longer than liveIdle whose thread
// has no listening service.
func reapLive(ctx context.Context) {
	reapOnce.Do(func() {
		go func() {
			tick := time.NewTicker(time.Minute)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
				closeAll(take(func(k string) bool {
					p := live.procs[k]
					thread, _, _ := strings.Cut(k, "\x00")
					return !p.Busy() && time.Since(p.IdleSince()) > liveIdle && len(ThreadServices(thread)) == 0
				}))
			}
		}()
	})
}

// stdioProc is a harness speaking a line protocol on stdio, kept open
// between turns. One turn at a time reads its stdout.
type stdioProc struct {
	key  string
	cmd  *exec.Cmd
	in   io.WriteCloser
	dead chan struct{}

	mu    sync.Mutex
	cur   *stdioTurn
	idle  time.Time
	turns int
	// state keeps what the provider needs between turns, such as its thread id.
	state map[string]string
}

// stdioTurn is one turn on a stdioProc.
type stdioTurn struct {
	handle   func(line []byte, emit func(Event))
	out      chan Event
	done     chan struct{}
	finished bool // guarded by the stdioProc mutex
}

func (p *stdioProc) Busy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cur != nil
}

func (p *stdioProc) IdleSince() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.idle
}

func (p *stdioProc) exited() bool {
	select {
	case <-p.dead:
		return true
	default:
		return false
	}
}

func (p *stdioProc) get(k string) string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state[k]
}

func (p *stdioProc) set(k, v string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state[k] = v
	p.mu.Unlock()
}

// Close ends stdin and, as a harness may wait for its background jobs, then
// stops its process group with them.
func (p *stdioProc) Close() {
	p.mu.Lock()
	_ = p.in.Close()
	p.mu.Unlock()
	select {
	case <-p.dead:
		return
	case <-time.After(2 * time.Second):
	}
	stopGroup(p.cmd, syscall.SIGTERM)
	select {
	case <-p.dead:
	case <-time.After(5 * time.Second):
		stopGroup(p.cmd, syscall.SIGKILL)
	}
}

// reuseStdio returns the open process of the thread when it can take a turn
// with the same settings, and closes it otherwise.
func reuseStdio(thread, provider, key string) *stdioProc {
	p, _ := liveGet(thread, provider).(*stdioProc)
	if p == nil || thread == "" {
		return nil
	}
	if p.key == key && !p.Busy() && !p.exited() {
		return p
	}
	liveDrop(thread, provider, p)
	go p.Close()
	return nil
}

// openStdio starts cmd, which must not be started yet, and keeps it for the
// thread. Each line of stdout goes to the turn running on it.
func openStdio(cmd *exec.Cmd, thread, provider, key string) (*stdioProc, error) {
	interruptible(cmd)
	cmd.WaitDelay = 10 * time.Second
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &limited{w: &stderr, n: 8 << 10}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &stdioProc{key: key, cmd: cmd, in: in, dead: make(chan struct{}), idle: time.Now(), state: map[string]string{}}
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64<<10), 32<<20)
		for scanner.Scan() {
			p.mu.Lock()
			t := p.cur
			p.mu.Unlock()
			if t != nil {
				t.handle(scanner.Bytes(), func(e Event) { p.emit(t, e) })
			}
		}
		err := cmd.Wait()
		close(p.dead)
		liveDrop(thread, provider, p)
		p.mu.Lock()
		t := p.cur
		p.mu.Unlock()
		if t != nil {
			msg := strings.TrimSpace(stderr.String())
			switch {
			case err != nil && msg == "":
				msg = err.Error()
			case msg == "":
				msg = "el agente terminó sin respuesta"
			}
			p.emit(t, Event{Done: true, Err: msg})
		}
	}()
	if thread != "" {
		liveSet(thread, provider, p)
	}
	return p, nil
}

// begin makes handle the reader of the next turn. Without a thread nothing
// can reuse the process, so it ends with the turn.
func (p *stdioProc) begin(thread string, handle func([]byte, func(Event))) *stdioTurn {
	t := &stdioTurn{handle: handle, out: make(chan Event, 64), done: make(chan struct{})}
	p.mu.Lock()
	p.cur = t
	p.turns++
	p.mu.Unlock()
	if thread == "" {
		go func() { <-t.done; p.Close() }()
	}
	return t
}

// abort drops a turn that could not start.
func (p *stdioProc) abort(t *stdioTurn) {
	p.mu.Lock()
	if p.cur == t {
		p.cur = nil
	}
	p.mu.Unlock()
}

// watch interrupts the turn when ctx ends: by protocol first, then, if the
// turn does not end within 5s, by closing the process.
func (p *stdioProc) watch(ctx context.Context, thread, provider string, t *stdioTurn, interrupt func()) {
	go func() {
		select {
		case <-t.done:
		case <-ctx.Done():
			interrupt()
			select {
			case <-t.done:
			case <-time.After(5 * time.Second):
				liveDrop(thread, provider, p)
				p.Close()
			}
		}
	}()
}

// emit delivers an event of turn t; its Done event ends the turn and frees
// the process for the next one.
func (p *stdioProc) emit(t *stdioTurn, e Event) {
	p.mu.Lock()
	if t.finished {
		p.mu.Unlock()
		return
	}
	if e.Done {
		t.finished = true
		if p.cur == t {
			p.cur = nil
			p.idle = time.Now()
		}
	}
	p.mu.Unlock()
	t.out <- e
	if e.Done {
		close(t.out)
		close(t.done)
	}
}

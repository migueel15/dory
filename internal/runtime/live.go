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

	"github.com/migueel15/dory/internal/harness/procpool"
)

// stdioProc is a harness speaking a line protocol on stdio, kept open
// between turns. One turn at a time reads its stdout.
type stdioProc struct {
	pool *procpool.Pool
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
func reuseStdio(pool *procpool.Pool, thread, provider, key string) *stdioProc {
	p, _ := pool.Get(thread, provider).(*stdioProc)
	if p == nil || thread == "" {
		return nil
	}
	if p.key == key && !p.Busy() && !p.exited() {
		return p
	}
	pool.Drop(thread, provider, p)
	go p.Close()
	return nil
}

// openStdio starts cmd, which must not be started yet, and keeps it for the
// thread. Each line of stdout goes to the turn running on it.
func openStdio(pool *procpool.Pool, cmd *exec.Cmd, thread, provider, key string) (*stdioProc, error) {
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
	p := &stdioProc{pool: pool, key: key, cmd: cmd, in: in, dead: make(chan struct{}), idle: time.Now(), state: map[string]string{}}
	if thread != "" {
		pool.Set(thread, provider, p)
	}
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
		pool.Drop(thread, provider, p)
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
				p.pool.Drop(thread, provider, p)
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

package kit

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/procpool"
)

// stdioProc is a harness speaking a line protocol on stdio, kept open
// between Turns. One turn at a time reads its stdout.
type StdioProc struct {
	pool  *procpool.Pool
	key   string
	cmd   *exec.Cmd
	Input io.WriteCloser
	dead  chan struct{}

	mu    sync.Mutex
	cur   *StdioTurn
	idle  time.Time
	Turns int
	// state keeps what the provider needs between Turns, such as its thread id.
	state map[string]string
}

// StdioTurn is one turn on a stdioProc.
type StdioTurn struct {
	handle   func(line []byte, emit func(harness.Event))
	Events   chan harness.Event
	done     chan struct{}
	finished bool // guarded by the stdioProc mutex
}

func (p *StdioProc) Busy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cur != nil
}

func (p *StdioProc) IdleSince() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.idle
}

func (p *StdioProc) exited() bool {
	select {
	case <-p.dead:
		return true
	default:
		return false
	}
}

func (p *StdioProc) Get(k string) string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state[k]
}

func (p *StdioProc) Set(k, v string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state[k] = v
	p.mu.Unlock()
}

// Close ends stdin and, as a harness may wait for its background jobs, then
// stops its process group with them.
func (p *StdioProc) Close() {
	p.mu.Lock()
	_ = p.Input.Close()
	p.mu.Unlock()
	select {
	case <-p.dead:
		return
	case <-time.After(2 * time.Second):
	}
	StopGroup(p.cmd, syscall.SIGTERM)
	select {
	case <-p.dead:
	case <-time.After(5 * time.Second):
		StopGroup(p.cmd, syscall.SIGKILL)
	}
}

// reuseStdio returns the open process of the thread when it can take a turn
// with the same settings, and closes it otherwise.
func ReuseStdio(pool *procpool.Pool, thread, provider, key string) *StdioProc {
	p, _ := pool.Get(thread, provider).(*StdioProc)
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
func OpenStdio(pool *procpool.Pool, cmd *exec.Cmd, thread, provider, key string) (*StdioProc, error) {
	Interruptible(cmd)
	cmd.WaitDelay = 10 * time.Second
	Input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = LimitWriter(&stderr, 8<<10)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &StdioProc{pool: pool, key: key, cmd: cmd, Input: Input, dead: make(chan struct{}), idle: time.Now(), state: map[string]string{}}
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
				t.handle(scanner.Bytes(), func(e harness.Event) { p.emit(t, e) })
			}
		}
		err := cmd.Wait()
		close(p.dead)
		pool.Drop(thread, provider, p)
		p.mu.Lock()
		t := p.cur
		p.mu.Unlock()
		if t != nil {
			p.emit(t, ProcessEnd(nil, err, strings.TrimSpace(stderr.String())))
		}
	}()
	return p, nil
}

// Begin makes handle the reader of the next turn. Without a thread nothing
// can reuse the process, so it ends with the turn.
func (p *StdioProc) Begin(thread string, handle func([]byte, func(harness.Event))) *StdioTurn {
	t := &StdioTurn{handle: handle, Events: make(chan harness.Event, 64), done: make(chan struct{})}
	p.mu.Lock()
	p.cur = t
	p.Turns++
	p.mu.Unlock()
	if thread == "" {
		go func() { <-t.done; p.Close() }()
	}
	return t
}

// Abort drops a turn that could not start.
func (p *StdioProc) Abort(t *StdioTurn) {
	p.mu.Lock()
	if p.cur == t {
		p.cur = nil
	}
	p.mu.Unlock()
}

// Watch interrupts the turn when ctx ends: by protocol first, then, if the
// turn does not end within 5s, by closing the process.
func (p *StdioProc) Watch(ctx context.Context, thread, provider string, t *StdioTurn, interrupt func()) {
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
func (p *StdioProc) emit(t *StdioTurn, e harness.Event) {
	_, ended := e.(harness.TurnEnded)
	p.mu.Lock()
	if t.finished {
		p.mu.Unlock()
		return
	}
	if ended {
		t.finished = true
		if p.cur == t {
			p.cur = nil
			p.idle = time.Now()
		}
	}
	p.mu.Unlock()
	t.Events <- e
	if ended {
		close(t.Events)
		close(t.done)
	}
}

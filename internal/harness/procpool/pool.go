// Package procpool owns the live harness processes and administration jobs of one node.
package procpool

import (
	"context"
	"sync"
	"time"
)

type Process interface {
	Busy() bool
	IdleSince() time.Time
	Close()
}

const IdleTimeout = 30 * time.Minute

type Key struct {
	Thread, Provider string
}

type processEntry struct {
	process  Process
	reusedAt time.Time
}

type Pool struct {
	mu          sync.Mutex
	procs       map[Key]processEntry
	jobsMu      sync.Mutex
	jobs        map[string]*Job
	hasServices func(string) bool
	now         func() time.Time
}

func New(hasServices func(string) bool) *Pool {
	return &Pool{procs: map[Key]processEntry{}, jobs: map[string]*Job{}, hasServices: hasServices, now: time.Now}
}

func (p *Pool) Get(thread, provider string) Process {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := Key{thread, provider}
	item, ok := p.procs[key]
	if ok {
		item.reusedAt = p.now()
		p.procs[key] = item
	}
	return item.process
}

func (p *Pool) Set(thread, provider string, process Process) {
	p.mu.Lock()
	p.procs[Key{thread, provider}] = processEntry{process: process}
	p.mu.Unlock()
}

// Drop removes process only if it is still the registered process.
func (p *Pool) Drop(thread, provider string, process Process) {
	p.mu.Lock()
	key := Key{thread, provider}
	if p.procs[key].process == process {
		delete(p.procs, key)
	}
	p.mu.Unlock()
}

func (p *Pool) Take(match func(Key, Process) bool) []Process {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []Process
	for k, item := range p.procs {
		if match(k, item.process) {
			out = append(out, item.process)
			delete(p.procs, k)
		}
	}
	return out
}

func closeProcesses(processes []Process) {
	var wg sync.WaitGroup
	for _, process := range processes {
		wg.Go(process.Close)
	}
	wg.Wait()
}

func (p *Pool) CloseThread(thread string) {
	closeProcesses(p.Take(func(k Key, _ Process) bool { return k.Thread == thread }))
}

func (p *Pool) CloseProcess(thread, provider string) {
	closeProcesses(p.Take(func(k Key, _ Process) bool { return k == (Key{thread, provider}) }))
}

func (p *Pool) CloseAll() {
	closeProcesses(p.Take(func(Key, Process) bool { return true }))
}

func (p *Pool) reap() {
	now := p.now()
	closeProcesses(p.Take(func(k Key, process Process) bool {
		return !process.Busy() && now.Sub(process.IdleSince()) > IdleTimeout && now.Sub(p.procs[k].reusedAt) > IdleTimeout &&
			(p.hasServices == nil || !p.hasServices(k.Thread))
	}))
}

// Reap closes processes idle past IdleTimeout on threads without services.
func (p *Pool) Reap(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			p.reap()
		}
	}
}

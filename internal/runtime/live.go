package runtime

import (
	"context"
	"strings"
	"sync"
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

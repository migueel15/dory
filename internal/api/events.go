package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/runtime"
)

// Change tells clients that an entity changed, so they refetch it.
type Change struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
}

// Events fans out data changes and thread updates to the open clients over
// server-sent events.
type Events struct {
	runtime *runtime.Manager
	mu      sync.Mutex
	subs    map[chan Change]struct{}
}

func NewEvents(rt *runtime.Manager) *Events {
	return &Events{runtime: rt, subs: map[chan Change]struct{}{}}
}

// Changed is called after every committed write, local or replicated.
func (e *Events) Changed(entity, id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ch := range e.subs {
		select {
		case ch <- Change{entity, id}:
		default:
		}
	}
}

// serve streams "change" events for data and "thread" events for thread
// updates. ?thread=<id> limits thread updates to one thread.
func (e *Events) serve(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}
	changes := make(chan Change, 256)
	e.mu.Lock()
	e.subs[changes] = struct{}{}
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.subs, changes)
		e.mu.Unlock()
	}()
	updates, stop := e.runtime.Subscribe(r.URL.Query().Get("thread"))
	defer stop()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": ready\n\n")
	flusher.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	ports := time.NewTicker(3 * time.Second)
	defer ports.Stop()
	threadID := r.URL.Query().Get("thread")
	lastPorts := ""
	send := func(kind string, v any) {
		raw, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, raw)
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case c := <-changes:
			send("change", c)
		case u := <-updates:
			send("thread", u)
		case <-ports.C:
			if threadID != "" {
				services := runtime.ThreadServices(threadID)
				raw, _ := json.Marshal(services)
				if string(raw) != lastPorts {
					lastPorts = string(raw)
					send("thread.ports", services)
				}
			}
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

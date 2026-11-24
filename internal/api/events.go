package api

import (
	"encoding/json"
	"fmt"
	"github.com/migueel15/dory/internal/apperr"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/runtime"
)

// Change tells clients that an entity changed, so they refetch it.
type Change struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
	// Data carries what a notification needs beyond the entity, e.g. which
	// device an agent opened.
	Data any `json:"data,omitempty"`
}

// Events fans out data changes and thread updates to the open clients over
// server-sent events.
type Events struct {
	runtime *runtime.Manager
	mu      sync.Mutex
	subs    map[chan Change]struct{}
	// epoch changes with every start of the node: a client that sees another
	// one has missed whatever happened in between and refetches everything.
	epoch string
}

func NewEvents(rt *runtime.Manager) *Events {
	return &Events{runtime: rt, subs: map[chan Change]struct{}{}, epoch: strconv.FormatInt(time.Now().UnixNano(), 36)}
}

// Changed is called after every committed write, local or replicated.
func (e *Events) Changed(entity, id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ch := range e.subs {
		select {
		case ch <- Change{Entity: entity, ID: id}:
		default:
		}
	}
}

// Notify tells the open clients about something that is not a stored change.
func (e *Events) Notify(c Change) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ch := range e.subs {
		select {
		case ch <- c:
		default:
		}
	}
}

// serve streams "change" events for data and "thread" events with the
// activity of the threads: hints to refetch, not the record of a thread (that
// is /api/threads/{id}/stream). It starts with a "hello" carrying the epoch.
func (e *Events) serve(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, r, apperr.New(apperr.Internal, "streaming unsupported"))
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
	updates, stop := e.runtime.Subscribe("")
	defer stop()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, ": ready\n\nevent: hello\ndata: {\"epoch\":%q}\n\n", e.epoch)
	flusher.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
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
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

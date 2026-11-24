package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/runtime"
)

// replayBatch is how many recorded events are read at once when a client
// resumes.
const replayBatch = 500

// streamThread is GET /api/threads/{id}/stream: the events of one thread over
// server-sent events. Durable events carry their number as the SSE id; a
// client that reconnects sends it back as Last-Event-ID (or ?after=) and gets
// exactly what it missed from the record, then the live events. Fragments of
// a text being written (event "delta") have no id and are not replayed. A
// subscriber that cannot keep up is closed rather than dropped events.
func (s *Server) streamThread(w http.ResponseWriter, r *http.Request, t data.Thread) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, r, apperr.New(apperr.Internal, "streaming unsupported"))
		return
	}
	after, resuming := -1, false
	for _, v := range []string{r.Header.Get("Last-Event-ID"), r.URL.Query().Get("after")} {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			after, resuming = n, true
			break
		}
	}
	// Subscribe first: an event is either in the replay or in the live channel.
	live, _, stop := s.Runtime.SubscribeEvents(t.ID)
	defer stop()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	send := func(e runtime.ThreadEvent) {
		raw, _ := json.Marshal(e)
		if e.Seq > 0 {
			fmt.Fprintf(w, "id: %d\n", e.Seq)
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", sseName(e), raw)
	}
	last := after
	if resuming {
		for {
			batch, err := s.Runtime.EventsAfter(r.Context(), t.ID, last, replayBatch)
			if err != nil {
				return
			}
			for _, e := range batch {
				send(e)
				last = e.Seq
			}
			if len(batch) < replayBatch {
				break
			}
		}
	} else {
		last = s.Runtime.EventSeq(r.Context(), t.ID)
		raw, _ := json.Marshal(map[string]int{"seq": last})
		fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", raw)
	}
	flusher.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e, open := <-live:
			if !open {
				// Closed for being slow: the client resumes from the record.
				return
			}
			if e.Seq != 0 && e.Seq <= last {
				continue
			}
			send(e)
			if e.Seq != 0 {
				last = e.Seq
			}
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func sseName(e runtime.ThreadEvent) string {
	if e.Type == runtime.EventDelta {
		return "delta"
	}
	return "thread"
}

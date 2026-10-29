package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/migueel15/dory/internal/service"
)

const (
	pollInterval = time.Second
	heartbeat    = 15 * time.Second
)

func search(svc service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "q is required")
			return
		}
		limit, err1 := intParam(r, "limit")
		offset, err2 := intParam(r, "offset")
		if err1 != nil || err2 != nil || limit > 1000 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid limit or offset")
			return
		}
		hits, err := svc.Search(r.Context(), q, int32(limit), int32(offset))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, hits)
	}
}

// streamEvents polls the audit log and emits each new event as an SSE message.
func streamEvents(svc service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, err := intParam(r, "after")
		if h := r.Header.Get("Last-Event-ID"); err == nil && h != "" {
			after, err = strconv.ParseInt(h, 10, 64)
			if err == nil && after < 0 {
				after, err = 0, fmt.Errorf("Last-Event-ID must be non-negative")
			}
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		kind := r.URL.Query().Get("entity_type")
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
			return
		}
		// Fail fast on bad parameters before committing to a 200 stream.
		events, err := svc.ListEvents(r.Context(), after, kind, 100)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		poll := time.NewTicker(pollInterval)
		defer poll.Stop()
		beat := time.NewTicker(heartbeat)
		defer beat.Stop()
		for {
			for _, e := range events {
				data, err := jsonLine(e)
				if err != nil {
					return
				}
				fmt.Fprintf(w, "id: %d\nevent: change\ndata: %s\n\n", e.ID, data)
				after = e.ID
			}
			flusher.Flush()
			full := len(events) == 100
			events = nil
			if full {
				// More pending: skip waiting.
				events, err = svc.ListEvents(r.Context(), after, kind, 100)
				if err != nil {
					return
				}
				continue
			}
			select {
			case <-r.Context().Done():
				return
			case <-beat.C:
				fmt.Fprint(w, ": keep-alive\n\n")
				flusher.Flush()
				continue
			case <-poll.C:
			}
			if events, err = svc.ListEvents(r.Context(), after, kind, 100); err != nil {
				return
			}
		}
	}
}

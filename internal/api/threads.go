package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

func writeT3Error(w http.ResponseWriter, err error) {
	if errors.Is(err, t3.ErrThreadNotFound) {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if errors.Is(err, t3.ErrInstanceUnavailable) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	writeError(w, http.StatusBadGateway, "upstream", err.Error())
}

func mountThreads(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor) {
	mux.HandleFunc("GET /api/t3/threads/{id}/context", func(w http.ResponseWriter, r *http.Request) {
		c, err := monitor.ThreadClient(r.Context(), r.PathValue("id"))
		if err != nil {
			writeT3Error(w, err)
			return
		}
		out, err := c.ThreadContext(r.Context(), r.PathValue("id"))
		if err != nil {
			writeT3Error(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("GET /api/t3/threads/{id}/stream", streamThread(svc, monitor))
	mux.HandleFunc("POST /api/t3/threads/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "text is required")
			return
		}
		id := r.PathValue("id")
		c, err := monitor.ThreadClient(r.Context(), id)
		if err == nil {
			err = c.SendMessage(r.Context(), id, body.Text)
		}
		if err != nil {
			writeT3Error(w, err)
			return
		}
		monitor.RefreshThread(id)
		audit(r, svc, id, map[string]any{"operation": "t3.message", "text": body.Text})
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/t3/threads/{id}/requests/{request_id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Decision string         `json:"decision"`
			Answers  map[string]any `json:"answers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid body")
			return
		}
		id, requestID := r.PathValue("id"), r.PathValue("request_id")
		c, err := monitor.ThreadClient(r.Context(), id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		// Re-read the request so the audit records what was really decided.
		tc, err := c.ThreadContext(r.Context(), id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		if tc.Pending == nil || tc.Pending.ID != requestID {
			writeError(w, http.StatusConflict, "conflict", "the request is no longer open")
			return
		}
		isQuestion := tc.Pending.Kind == "user_input"
		switch {
		case isQuestion && len(body.Answers) > 0:
			err = c.RespondUserInput(r.Context(), id, requestID, body.Answers)
		case !isQuestion && body.Decision == t3.DecisionDecline, !isQuestion && body.Decision == t3.DecisionAccept && tc.Pending.Detail != "":
			err = c.RespondApproval(r.Context(), id, requestID, body.Decision)
		default:
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid response for this request")
			return
		}
		if err != nil {
			writeT3Error(w, err)
			return
		}
		monitor.RefreshThread(id)
		audit(r, svc, id, map[string]any{"operation": "t3.respond", "request": tc.Pending, "decision": body.Decision, "answers": body.Answers})
		w.WriteHeader(http.StatusNoContent)
	})
}

// audit runs after T3 accepted the action, so a failure here is only logged.
func audit(r *http.Request, svc service.Service, threadID string, change map[string]any) {
	if err := svc.RecordEvent(r.Context(), actor(r), "t3_thread", threadID, change); err != nil {
		logError(err)
	}
}

func logError(err error) { log.Printf("api: audit t3 action: %v", err) }

// threadDebounce groups the events of a running turn into one refetch.
const threadDebounce = 250 * time.Millisecond

// threadClient reaches a thread the monitor reports or, as with definition
// threads (ADR-004), one Dory linked to an item on an available instance.
func threadClient(ctx context.Context, svc service.Service, monitor *t3.Monitor, id string) (t3.Client, error) {
	c, err := monitor.ThreadClient(ctx, id)
	if err == nil {
		return c, nil
	}
	links, lerr := svc.ItemThreads().List(ctx, service.Filter{Fields: map[string]any{"thread_id": id}, Limit: 1})
	if lerr != nil || len(links) == 0 {
		return t3.Client{}, err
	}
	return monitor.InstanceClient(ctx, links[0].InstanceID)
}

// streamThread follows one thread while the client keeps the request open and
// emits "event: thread" after each burst of changes, so the open conversation
// refetches without polling. It ends when the client leaves or T3 drops it;
// the browser's EventSource reconnects on its own.
func streamThread(svc service.Service, monitor *t3.Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, err := threadClient(r.Context(), svc, monitor, id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		signal := make(chan struct{}, 1)
		failed := make(chan error, 1)
		go func() { failed <- t3.FollowThread(ctx, c, id, signal) }()
		beat := time.NewTicker(heartbeat)
		defer beat.Stop()
		var debounce <-chan time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-failed:
				if ctx.Err() == nil {
					log.Printf("api: thread stream %s: %v", id, err)
				}
				return
			case <-beat.C:
				fmt.Fprint(w, ": keep-alive\n\n")
				flusher.Flush()
			case <-signal:
				if debounce == nil {
					debounce = time.After(threadDebounce)
				}
			case <-debounce:
				debounce = nil
				fmt.Fprintf(w, "event: thread\ndata: {\"thread_id\":%q}\n\n", id)
				flusher.Flush()
			}
		}
	}
}

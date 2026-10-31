package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

func writeT3Error(w http.ResponseWriter, err error) {
	if errors.Is(err, t3.ErrThreadNotFound) {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
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

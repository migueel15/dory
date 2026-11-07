package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// threadViewOut is a thread as the chat view shows it, with the Dory items
// linked to it and the instance whose model catalog the composer offers.
type threadViewOut struct {
	t3.ThreadView
	InstanceID string               `json:"instance_id"`
	Items      []service.ItemThread `json:"items"`
	// HistoryCursor loads older messages with /history; empty when none.
	HistoryCursor string `json:"history_cursor"`
}

// maxAttachmentBytes is T3's own limit for a file attachment.
const maxAttachmentBytes = 50 << 20

// mountThreadView serves the chat view of one thread. Queue and steer are T3's
// own, so the queue is shared with T3 Code's UI.
func mountThreadView(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor) {
	// act resolves the thread, decodes the body and runs one T3 command.
	act := func(operation string, body any, run func(r *http.Request, c t3.Client, id string) error) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if body != nil {
				if err := json.NewDecoder(r.Body).Decode(body); err != nil {
					writeError(w, http.StatusBadRequest, "invalid_argument", "invalid body")
					return
				}
			}
			id := r.PathValue("id")
			c, err := threadClient(r.Context(), svc, monitor, id)
			if err == nil {
				err = run(r, c, id)
			}
			if err != nil {
				writeT3Error(w, err)
				return
			}
			monitor.RefreshThread(id)
			audit(r, svc, id, map[string]any{"operation": operation, "run_id": r.PathValue("run_id"), "body": body})
			w.WriteHeader(http.StatusNoContent)
		}
	}

	mux.HandleFunc("GET /api/t3/threads/{id}/view", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, err := threadClient(r.Context(), svc, monitor, id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		v, err := c.ThreadView(r.Context(), id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		links, err := svc.ItemThreads().List(r.Context(), service.Filter{Fields: map[string]any{"thread_id": id}, Limit: 20})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		out := threadViewOut{ThreadView: v, Items: links}
		// T3 sends only the latest turns; older ones come page by page.
		if cursor, err := c.HistoryCursor(r.Context(), id); err == nil {
			out.HistoryCursor = cursor
		}
		if inst, ok := monitor.Locate(id); ok {
			out.InstanceID = inst
		} else if len(links) > 0 {
			out.InstanceID = links[0].InstanceID
		}
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("GET /api/t3/threads/{id}/diff", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, err := threadClient(r.Context(), svc, monitor, id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		diff, err := c.ThreadDiff(r.Context(), id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"diff": diff})
	})
	mux.HandleFunc("POST /api/t3/threads/{id}/send", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Text         string          `json:"text"`
			Mode         string          `json:"mode"`
			ProviderID   string          `json:"provider_id"`
			Model        string          `json:"model"`
			EffortOption string          `json:"effort_option"`
			Effort       string          `json:"effort"`
			Attachments  []t3.Attachment `json:"attachments"`
		}
		act("t3.send", &b, func(r *http.Request, c t3.Client, id string) error {
			return c.Send(r.Context(), id, t3.SendInput{Text: b.Text, Mode: b.Mode, ProviderID: b.ProviderID, Model: b.Model, EffortOption: b.EffortOption, Effort: b.Effort, Attachments: b.Attachments})
		})(w, r)
	})
	// Attachments go up one at a time as the raw body, with the file name in
	// ?name= and its type in Content-Type; the reply goes back in /send.
	mux.HandleFunc("POST /api/t3/threads/{id}/attachments", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAttachmentBytes))
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "invalid_argument", "attachment too large (max 50 MB)")
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			name = "adjunto"
		}
		c, err := threadClient(r.Context(), svc, monitor, id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		a, err := c.Upload(r.Context(), name, r.Header.Get("Content-Type"), data)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	})
	mux.HandleFunc("GET /api/t3/threads/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		cursor := r.URL.Query().Get("cursor")
		if cursor == "" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "cursor is required")
			return
		}
		c, err := threadClient(r.Context(), svc, monitor, id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		page, err := c.History(r.Context(), id, cursor)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	})
	mux.HandleFunc("PATCH /api/t3/threads/{id}/queue/{run_id}", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Text string `json:"text"`
		}
		act("t3.queue.edit", &b, func(r *http.Request, c t3.Client, id string) error {
			return c.EditQueued(r.Context(), id, r.PathValue("run_id"), b.Text)
		})(w, r)
	})
	mux.HandleFunc("POST /api/t3/threads/{id}/queue/{run_id}/reorder", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			BeforeRunID string `json:"before_run_id"`
		}
		act("t3.queue.reorder", &b, func(r *http.Request, c t3.Client, id string) error {
			return c.ReorderQueued(r.Context(), id, r.PathValue("run_id"), b.BeforeRunID)
		})(w, r)
	})
	mux.HandleFunc("POST /api/t3/threads/{id}/queue/{run_id}/steer", act("t3.queue.steer", nil, func(r *http.Request, c t3.Client, id string) error {
		v, err := c.ThreadView(r.Context(), id)
		if err != nil {
			return err
		}
		if v.ActiveRunID == "" {
			// Nothing to steer: the queue is already starting it.
			return nil
		}
		return c.SteerQueued(r.Context(), id, r.PathValue("run_id"), v.ActiveRunID)
	}))
	mux.HandleFunc("POST /api/t3/threads/{id}/interrupt", act("t3.interrupt", nil, func(r *http.Request, c t3.Client, id string) error {
		v, err := c.ThreadView(r.Context(), id)
		if err != nil {
			return err
		}
		if v.ActiveRunID == "" {
			return nil // already stopped
		}
		return c.Interrupt(r.Context(), id, v.ActiveRunID)
	}))
	mux.HandleFunc("POST /api/t3/threads/{id}/queue/resume", act("t3.queue.resume", nil, func(r *http.Request, c t3.Client, id string) error {
		return c.ResumeQueue(r.Context(), id)
	}))
	mux.HandleFunc("DELETE /api/t3/threads/{id}/queue/{run_id}", act("t3.queue.cancel", nil, func(r *http.Request, c t3.Client, id string) error {
		return c.CancelQueued(r.Context(), id, r.PathValue("run_id"))
	}))
	mux.HandleFunc("PATCH /api/t3/threads/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			InteractionMode string `json:"interaction_mode"`
			RuntimeMode     string `json:"runtime_mode"`
		}
		act("t3.settings", &b, func(r *http.Request, c t3.Client, id string) error {
			if b.InteractionMode != "" {
				if err := c.SetInteractionMode(r.Context(), id, b.InteractionMode); err != nil {
					return err
				}
			}
			if b.RuntimeMode != "" {
				return c.SetRuntimeMode(r.Context(), id, b.RuntimeMode)
			}
			return nil
		})(w, r)
	})
}

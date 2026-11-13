package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// threadAction is one organization command on a thread, as in T3 Code's
// sidebar: rename, archive, delete, pin, settle, snooze, read state, fork, stop.
type threadAction struct {
	Action string `json:"action"`
	Title  string `json:"title,omitempty"`
	Until  string `json:"until,omitempty"`
	RunID  string `json:"run_id,omitempty"`
}

func mountThreadActions(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor) {
	// Every thread of every available instance, settled, archived and
	// subagents included, from the monitor's last read (no call to T3).
	mux.HandleFunc("GET /api/t3/threads", func(w http.ResponseWriter, r *http.Request) {
		writeJSONETag(w, r, monitor.AllThreads())
	})
	mux.HandleFunc("POST /api/t3/threads/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		var b threadAction
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid body")
			return
		}
		id := r.PathValue("id")
		c, err := threadClient(r.Context(), svc, monitor, id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		ctx := r.Context()
		var forked string
		switch b.Action {
		case "rename":
			err = c.RenameThread(ctx, id, b.Title)
		case "archive":
			err = c.ArchiveThread(ctx, id)
		case "unarchive":
			err = c.UnarchiveThread(ctx, id)
		case "delete":
			err = c.DeleteThread(ctx, id)
		case "pin":
			err = c.PinThread(ctx, id)
		case "unpin":
			err = c.UnpinThread(ctx, id)
		case "settle":
			err = c.SettleThread(ctx, id)
		case "unsettle":
			err = c.UnsettleThread(ctx, id)
		case "snooze":
			var until time.Time
			if until, err = time.Parse(time.RFC3339, b.Until); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_argument", "until must be an RFC 3339 time")
				return
			}
			err = c.SnoozeThread(ctx, id, until)
		case "unsnooze":
			err = c.UnsnoozeThread(ctx, id)
		case "read":
			err = c.VisitThread(ctx, id)
		case "unread":
			err = c.MarkThreadUnread(ctx, id)
		case "stop":
			err = c.StopThread(ctx, id)
		case "fork":
			run := b.RunID
			if run == "" {
				if th, ok := monitor.Thread(id); ok {
					run = th.RunID
				}
			}
			if run == "" {
				writeError(w, http.StatusBadRequest, "invalid_argument", "the thread has no turn to fork from")
				return
			}
			forked, err = c.ForkThread(ctx, id, run, b.Title)
		default:
			writeError(w, http.StatusBadRequest, "invalid_argument", "unknown action")
			return
		}
		if err != nil {
			writeT3Error(w, err)
			return
		}
		// "read" happens on every open: the shell subscription picks it up, and
		// it is not worth an audit row or a fresh read of the conversation.
		if b.Action != "read" {
			views.forget(id)
			monitor.RefreshThread(id)
			audit(r, svc, id, map[string]any{"operation": "t3." + b.Action, "title": b.Title, "until": b.Until, "forked": forked})
		}
		if forked != "" {
			writeJSON(w, http.StatusOK, map[string]string{"thread_id": forked})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

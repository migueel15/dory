package api

import (
	"bytes"
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
		// When an item of the thread names a branch checked out on this machine,
		// Dory diffs that worktree itself: it also covers worktrees T3 does not know.
		if links, err := svc.ItemThreads().List(r.Context(), service.Filter{Fields: map[string]any{"thread_id": id}, Limit: 20}); err == nil {
			if out, ok := gitChanges(r.Context(), svc, links); ok {
				writeJSONETag(w, r, out)
				return
			}
		}
		diff, err := c.ThreadDiff(r.Context(), id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		writeJSONETag(w, r, localChanges{Diff: diff, Source: "t3"})
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
			Skills       []string        `json:"skills"`
			Mentions     []string        `json:"mentions"`
		}
		act("t3.send", &b, func(r *http.Request, c t3.Client, id string) error {
			return c.Send(r.Context(), id, t3.SendInput{Text: b.Text, Mode: b.Mode, ProviderID: b.ProviderID, Model: b.Model, EffortOption: b.EffortOption, Effort: b.Effort, Attachments: b.Attachments, Skills: b.Skills, Mentions: b.Mentions})
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
	// Images the agent shows by local path (Markdown ![](/abs/path.png)),
	// proxied from the thread's machine like T3 Code does. Only images pass.
	mux.HandleFunc("GET /api/t3/threads/{id}/media", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" || len(path) > 1024 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "path is required")
			return
		}
		c, err := threadClient(r.Context(), svc, monitor, r.PathValue("id"))
		if err != nil {
			writeT3Error(w, err)
			return
		}
		resp, err := c.OpenMedia(r.Context(), r.PathValue("id"), path)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		defer resp.Body.Close()
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		if !strings.HasPrefix(ct, "image/") || strings.Contains(ct, "svg") {
			writeError(w, http.StatusUnsupportedMediaType, "invalid_argument", "only images are shown")
			return
		}
		for _, h := range []string{"Content-Type", "Content-Length"} {
			if v := resp.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		io.Copy(w, io.LimitReader(resp.Body, maxAttachmentBytes))
	})
	// Attachments of a thread's messages, proxied from T3 for previews.
	mux.HandleFunc("GET /api/t3/threads/{id}/attachments/{aid}", func(w http.ResponseWriter, r *http.Request) {
		c, err := threadClient(r.Context(), svc, monitor, r.PathValue("id"))
		if err != nil {
			writeT3Error(w, err)
			return
		}
		q := r.URL.Query()
		resp, err := c.Open(r.Context(), r.PathValue("aid"), q.Get("name"), q.Get("mime"))
		if err != nil {
			writeT3Error(w, err)
			return
		}
		defer resp.Body.Close()
		// Content-Disposition is dropped: Dory always shows the file inline.
		for _, h := range []string{"Content-Type", "Content-Length"} {
			if v := resp.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Markup must not run as Dory's origin; images and PDFs keep their viewers.
		// Rendered pages may run their own scripts (charts), but in an opaque origin.
		if ct := strings.ToLower(resp.Header.Get("Content-Type")); strings.Contains(ct, "html") || strings.Contains(ct, "svg") || strings.Contains(ct, "xml") {
			w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-popups allow-popups-to-escape-sandbox")
		}
		// ?fit=1 adds a script that reports the page height, so the chat sizes
		// the frame to it (a sandboxed frame cannot be measured from outside).
		if q.Get("fit") == "1" && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "html") {
			body, err := io.ReadAll(io.LimitReader(resp.Body, maxAttachmentBytes))
			if err != nil {
				writeError(w, http.StatusBadGateway, "unavailable", "could not read the page")
				return
			}
			w.Header().Del("Content-Length")
			w.Write(withHeightReport(body))
			return
		}
		io.Copy(w, resp.Body)
	})
	// Answers a secret request. The value goes straight to T3: it is never
	// logged, audited or stored by Dory.
	mux.HandleFunc("POST /api/t3/threads/{id}/secrets/{item}", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Secret  string `json:"secret"`
			Decline bool   `json:"decline"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&b); err != nil || (!b.Decline && strings.TrimSpace(b.Secret) == "") {
			writeError(w, http.StatusBadRequest, "invalid_argument", "secret or decline is required")
			return
		}
		id := r.PathValue("id")
		c, err := threadClient(r.Context(), svc, monitor, id)
		if err == nil {
			err = c.AnswerSecret(r.Context(), id, r.PathValue("item"), b.Secret, b.Decline)
		}
		b.Secret = ""
		if err != nil {
			writeT3Error(w, err)
			return
		}
		monitor.RefreshThread(id)
		audit(r, svc, id, map[string]any{"operation": "t3.secret", "item": r.PathValue("item"), "declined": b.Decline})
		w.WriteHeader(http.StatusNoContent)
	})
	// Files and folders for @ mentions. The worktree Dory found with git wins over
	// T3's folder, so threads in hand-made worktrees search the right tree.
	mux.HandleFunc("GET /api/t3/threads/{id}/entries", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, err := threadClient(r.Context(), svc, monitor, id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		cwd, err := threadFolder(r, svc, c, id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		entries, err := c.SearchEntries(r.Context(), cwd, r.URL.Query().Get("q"))
		if err != nil {
			writeT3Error(w, err)
			return
		}
		if entries == nil {
			entries = []t3.ProjectEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
	})
	// The / commands and $ skills for the composer: the repository's own (T3's
	// scan of the thread folder) plus the provider's global ones.
	mux.HandleFunc("GET /api/t3/threads/{id}/composer", func(w http.ResponseWriter, r *http.Request) {
		id, provider := r.PathValue("id"), r.URL.Query().Get("provider")
		if provider == "" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "provider is required")
			return
		}
		c, err := threadClient(r.Context(), svc, monitor, id)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		cwd, _ := threadFolder(r, svc, c, id)
		commands, skills, err := c.ComposerEntries(r.Context(), provider, cwd)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"commands": commands, "skills": skills})
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

// heightReport posts the document height to the parent on every change.
const heightReport = `<script>(function(){var last=0;function send(){var d=document.documentElement,b=document.body;` +
	`var h=Math.ceil(Math.max(d.scrollHeight,b?b.scrollHeight:0));if(h!==last){last=h;parent.postMessage({doryFrameHeight:h},"*")}}` +
	`addEventListener("load",send);if(window.ResizeObserver){new ResizeObserver(send).observe(document.documentElement)}setTimeout(send,0)})()</script>`

// withHeightReport inserts heightReport before </body>, or at the end.
func withHeightReport(page []byte) []byte {
	i := bytes.LastIndex(bytes.ToLower(page), []byte("</body>"))
	if i < 0 {
		return append(page, heightReport...)
	}
	out := append([]byte{}, page[:i]...)
	out = append(out, heightReport...)
	return append(out, page[i:]...)
}

// threadFolder is where a thread works: the worktree Dory finds with git from
// its items' branches, else the folder T3 knows (its worktree or project root).
func threadFolder(r *http.Request, svc service.Service, c t3.Client, id string) (string, error) {
	if links, err := svc.ItemThreads().List(r.Context(), service.Filter{Fields: map[string]any{"thread_id": id}, Limit: 20}); err == nil {
		for _, b := range itemBranches(r.Context(), svc, links) {
			if dir := findWorktree(r.Context(), svc, b); dir != "" {
				return dir, nil
			}
		}
	}
	return c.ThreadCwd(r.Context(), id)
}

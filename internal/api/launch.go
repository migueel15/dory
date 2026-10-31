package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

const (
	roleTriage         = "triage"
	roleImplementation = "implementation"
)

// launchPrompt gives the agent the item and what it may do with it.
func launchPrompt(item service.Item, role string, worktree bool) string {
	task := fmt.Sprintf("Item de Dory %s\nTítulo: %s\nTexto original: %s\nDescripción:\n%s\n\n", item.ID, item.Title, item.RawText, item.BodyMd)
	if role == roleTriage {
		return task + "Analiza esta idea y propón título, descripción, proyecto y feature en este hilo. No cambies nada en Dory ni en el código: el usuario decide si acepta la propuesta."
	}
	task += "Implementa este item siguiendo $dory-workflow; ya está en in_progress y vinculado a este hilo."
	if worktree {
		// Dory's token cannot create worktrees (no source-control:write), so the agent does.
		task += fmt.Sprintf(" Antes de tocar código crea con git un worktree nuevo desde main, en la rama dory/%s, y trabaja solo allí.", item.ID[:8])
	}
	return task
}

func mountLaunch(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor) {
	mux.HandleFunc("GET /api/t3/instances/{id}/catalog", func(w http.ResponseWriter, r *http.Request) {
		c, err := monitor.InstanceClient(r.Context(), r.PathValue("id"))
		if err != nil {
			writeT3Error(w, err)
			return
		}
		out, err := c.Catalog(r.Context())
		if err != nil {
			writeT3Error(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("POST /api/items/{id}/launch", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			InstanceID string `json:"instance_id"`
			ProjectID  string `json:"project_id"`
			Provider   string `json:"provider"`
			Model      string `json:"model"`
			Role       string `json:"role"`
			Worktree   bool   `json:"worktree"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.InstanceID == "" || body.ProjectID == "" || body.Provider == "" || body.Model == "" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "instance_id, project_id, provider and model are required")
			return
		}
		item, err := svc.Items().Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "item not found")
			return
		}
		// Only a ready item can be implemented: the jump is always deliberate.
		valid := body.Role == roleTriage && (item.Status == "inbox" || item.Status == "triaged") || body.Role == roleImplementation && item.Status == "ready"
		if !valid {
			writeError(w, http.StatusConflict, "conflict", "this item cannot be launched with that role in its current status")
			return
		}
		c, err := monitor.InstanceClient(r.Context(), body.InstanceID)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		threadID, err := c.Launch(r.Context(), t3.LaunchInput{
			ProjectID: body.ProjectID, Title: item.Title, Provider: body.Provider, Model: body.Model,
			Prompt: launchPrompt(item, body.Role, body.Worktree),
		})
		if threadID == "" {
			writeT3Error(w, err)
			return
		}
		// The thread exists from here on, so keep it linked even if its turn failed.
		link := service.ItemThread{ItemID: item.ID, InstanceID: body.InstanceID, ThreadID: threadID, Role: body.Role}
		if _, lerr := svc.ItemThreads().Put(r.Context(), actor(r), link); lerr != nil {
			logError(lerr)
		}
		if err != nil {
			writeT3Error(w, err)
			return
		}
		if body.Role == roleImplementation {
			item.Status = "in_progress"
			if _, err := svc.Items().Put(r.Context(), actor(r), item); err != nil {
				logError(err)
			}
		}
		audit(r, svc, threadID, map[string]any{"operation": "t3.launch", "item_id": item.ID, "role": body.Role, "model": body.Model})
		writeJSON(w, http.StatusCreated, map[string]string{"thread_id": threadID})
	})
}

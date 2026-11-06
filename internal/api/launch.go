package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// roleImplementation is the only role launched here; definition threads
// (refine) are started from the Define view through /refine.
const roleImplementation = "implementation"

// launchPrompt gives the agent the item and what it may do with it.
func launchPrompt(item service.Item, worktree bool, base string) string {
	task := fmt.Sprintf("Item de Dory %s\nTítulo: %s\nTexto original: %s\nDescripción:\n%s\n\n", item.ID, item.Title, item.RawText, item.BodyMd)
	task += "Implementa este item siguiendo $dory-workflow; ya está en in_progress y vinculado a este hilo."
	if worktree {
		// Dory's token cannot create worktrees (no source-control:write), so the agent does.
		from := "la rama por defecto del repositorio (origin/HEAD)"
		if base != "" {
			from = "la rama " + base
		}
		task += fmt.Sprintf(" Antes de tocar código actualiza el remoto y crea con git un worktree nuevo desde %s, en ~/.t3/worktrees/<repo>/dory-%s (sustituye <repo> por el nombre de la carpeta del repositorio principal), en la rama dory/%s, y trabaja solo allí.", from, item.ID[:8], item.ID[:8])
		task += " Tras el merge de la PR, vuelve al checkout principal y elimina el worktree con git worktree remove y la rama local con git branch -d, si es posible y sin cambios pendientes. No fuerces la limpieza; si falla o no puede hacerse, explica el motivo en este hilo."
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
			BaseBranch string `json:"base_branch"`
			// EffortOption and Effort set the reasoning effort; empty keeps the default.
			EffortOption string `json:"effort_option"`
			Effort       string `json:"effort"`
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
		if body.Role == "" {
			body.Role = roleImplementation
		}
		if body.Role != roleImplementation {
			writeError(w, http.StatusBadRequest, "invalid_argument", "only the implementation role is launched here; define items with /refine")
			return
		}
		// Only a ready item can be implemented: the jump is always deliberate.
		if item.Status != "ready" {
			writeError(w, http.StatusConflict, "conflict", "only a ready item can be sent to implementation")
			return
		}
		c, err := monitor.InstanceClient(r.Context(), body.InstanceID)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		threadID, err := c.Launch(r.Context(), t3.LaunchInput{
			ProjectID: body.ProjectID, Title: item.Title, Provider: body.Provider, Model: body.Model, EffortOption: body.EffortOption, Effort: body.Effort,
			Prompt: launchPrompt(item, body.Worktree, strings.TrimSpace(body.BaseBranch)),
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
		item.Status = "in_progress"
		if _, err := svc.Items().Put(r.Context(), actor(r), item); err != nil {
			logError(err)
		}
		audit(r, svc, threadID, map[string]any{"operation": "t3.launch", "item_id": item.ID, "role": body.Role, "model": body.Model, "effort": body.Effort})
		writeJSON(w, http.StatusCreated, map[string]string{"thread_id": threadID})
	})
}

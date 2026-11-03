package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

const (
	roleTriage         = "triage"
	roleImplementation = "implementation"
)

// triagePrompt lets the agent define the item in Dory, but stops at ready:
// implementing it stays a deliberate click of the user.
const triagePrompt = `Analiza esta idea consultando el código y las herramientas MCP de Dory (session_start, get_project). No cambies el código ni implementes nada.

Si la idea queda clara y sin dudas que cambien el resultado, déjala definida en Dory en este mismo turno:
1. Con update_item pon un título claro, body_md con objetivo, alcance y cómo probar, tags y project_id.
2. Si encaja en una feature existente del proyecto, asigna su feature_id. Si es una capacidad nueva, créala con upsert_feature y asígnala. Los bugs y tareas menores no necesitan feature.
3. Con move_item pásala a ready. No la lleves más allá: el usuario decide cuándo implementarla.

Si tienes dudas que cambien el resultado, guarda solo lo que esté claro, pásala a triaged y plantea las dudas en este hilo.

Termina con un resumen breve de lo que has dejado en Dory.`

// launchPrompt gives the agent the item and what it may do with it.
func launchPrompt(item service.Item, role string, worktree bool, base string) string {
	task := fmt.Sprintf("Item de Dory %s\nTítulo: %s\nTexto original: %s\nDescripción:\n%s\n\n", item.ID, item.Title, item.RawText, item.BodyMd)
	if role == roleTriage {
		return task + triagePrompt
	}
	task += "Implementa este item siguiendo $dory-workflow; ya está en in_progress y vinculado a este hilo."
	if worktree {
		// Dory's token cannot create worktrees (no source-control:write), so the agent does.
		from := "la rama por defecto del repositorio (origin/HEAD)"
		if base != "" {
			from = "la rama " + base
		}
		task += fmt.Sprintf(" Antes de tocar código actualiza el remoto y crea con git un worktree nuevo desde %s, en la rama dory/%s, y trabaja solo allí.", from, item.ID[:8])
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
			Prompt: launchPrompt(item, body.Role, body.Worktree, strings.TrimSpace(body.BaseBranch)),
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

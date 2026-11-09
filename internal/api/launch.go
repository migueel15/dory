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
func launchPrompt(item service.Item, worktree string) string {
	task := fmt.Sprintf("Item de Dory %s\nTítulo: %s\nTexto original: %s\nDescripción:\n%s\n\n", item.ID, item.Title, item.RawText, item.BodyMd)
	task += "Implementa este item siguiendo $dory-workflow; ya está en in_progress y vinculado a este hilo."
	if worktree != "" {
		task += fmt.Sprintf(" Ya trabajas en un worktree en %s. La rama provisional inicial es %s; si aún estás en ella, antes del primer commit renómbrala con git branch -m <nombre> según las skills de rama del proyecto. Si el worktree se ha reutilizado y la rama ya está renombrada, conserva esa rama. Antes de empezar haz git fetch y comprueba que partes de la base actual. No crees otro worktree.", worktree, provisionalBranch(item))
		task += " Tras el merge de la PR, Dory elimina el worktree; no lo borres tú."
	}
	return task
}

// worktreeRefPrefix marks the item ref that records the worktree Dory created.
const worktreeRefPrefix = "worktree:"

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
	// A free thread with no item: with text it starts the first turn, without
	// it the chat opens empty. An item can be linked later through /api/item-threads.
	mux.HandleFunc("POST /api/t3/threads", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			InstanceID   string `json:"instance_id"`
			ProjectID    string `json:"project_id"`
			Provider     string `json:"provider"`
			Model        string `json:"model"`
			EffortOption string `json:"effort_option"`
			Effort       string `json:"effort"`
			Text         string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.InstanceID == "" || body.ProjectID == "" || body.Provider == "" || body.Model == "" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "instance_id, project_id, provider and model are required")
			return
		}
		c, err := monitor.InstanceClient(r.Context(), body.InstanceID)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		var threadID string
		if strings.TrimSpace(body.Text) == "" {
			threadID, err = c.CreateThread(r.Context(), t3.CreateThreadInput{
				ProjectID: body.ProjectID, Title: quickTitle(""), InstanceID: body.Provider, Model: body.Model,
				EffortOption: body.EffortOption, Effort: body.Effort,
			})
		} else {
			threadID, err = c.Launch(r.Context(), t3.LaunchInput{
				ProjectID: body.ProjectID, Title: quickTitle(body.Text), Provider: body.Provider, Model: body.Model,
				EffortOption: body.EffortOption, Effort: body.Effort, Prompt: body.Text,
			})
		}
		if threadID == "" {
			writeT3Error(w, err)
			return
		}
		monitor.Refresh(body.InstanceID)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		audit(r, svc, threadID, map[string]any{"operation": "t3.thread.create", "model": body.Model, "effort": body.Effort})
		writeJSON(w, http.StatusCreated, map[string]string{"thread_id": threadID})
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
		if !sameRepository(w, r, svc, c, item, body.ProjectID) {
			return
		}
		// T3 creates the worktree on a provisional branch so its diff panel works from the
		// first edit; Dory cannot, because it has no write access to the repository.
		root, worktree := "", ""
		createdWorktree := false
		if body.Worktree {
			ref := "origin/HEAD"
			if base := strings.TrimSpace(body.BaseBranch); base != "" {
				ref = "origin/" + strings.TrimPrefix(base, "origin/")
			}
			if root, err = c.ProjectRoot(r.Context(), body.ProjectID); err != nil {
				writeT3Error(w, err)
				return
			}
			worktree, createdWorktree = itemWorktree(item)
			if createdWorktree {
				if worktree, err = c.CreateWorktree(r.Context(), root, ref, provisionalBranch(item)); err != nil {
					writeT3Error(w, err)
					return
				}
			}
		}
		threadID, err := c.Launch(r.Context(), t3.LaunchInput{
			ProjectID: body.ProjectID, Title: item.Title, Provider: body.Provider, Model: body.Model, EffortOption: body.EffortOption, Effort: body.Effort,
			WorktreePath: worktree, Prompt: launchPrompt(item, worktree),
		})
		if threadID == "" {
			if createdWorktree {
				if rerr := c.RemoveWorktree(r.Context(), root, worktree); rerr != nil {
					logError(rerr)
				}
			}
			writeT3Error(w, err)
			return
		}
		monitor.Refresh(body.InstanceID)
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
		if createdWorktree {
			item.Refs = append(item.Refs, worktreeRefPrefix+worktree)
		}
		if _, err := svc.Items().Put(r.Context(), actor(r), item); err != nil {
			logError(err)
		}
		audit(r, svc, threadID, map[string]any{"operation": "t3.launch", "item_id": item.ID, "role": body.Role, "model": body.Model, "effort": body.Effort})
		writeJSON(w, http.StatusCreated, map[string]string{"thread_id": threadID})
	})
}

// sameRepository allows working on an item only from a T3 project of the same
// repository as the item's Dory project. Items without a project, or whose
// project has no repository yet, are not restricted. It writes the error itself.
func sameRepository(w http.ResponseWriter, r *http.Request, svc service.Service, c t3.Client, item service.Item, t3Project string) bool {
	if item.ProjectID == nil || *item.ProjectID == "" {
		return true
	}
	repos, err := svc.Repositories().List(r.Context(), service.Filter{Fields: map[string]any{"project_id": *item.ProjectID}, Limit: 100})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return false
	}
	if len(repos) == 0 {
		return true
	}
	catalog, err := c.Catalog(r.Context())
	if err != nil {
		writeT3Error(w, err)
		return false
	}
	for _, p := range catalog.Projects {
		if p.ID != t3Project || p.Repo == "" {
			continue
		}
		for _, repo := range repos {
			if t3.RepoKey(repo.RemoteUrl) == t3.RepoKey(p.Repo) {
				return true
			}
		}
	}
	writeError(w, http.StatusBadRequest, "invalid_argument", "the T3 project must have the same repository as the item's Dory project")
	return false
}

// quickTitle names a free thread after the first line of its question.
func quickTitle(text string) string {
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0])
	if r := []rune(line); len(r) > 60 {
		line = string(r[:59]) + "…"
	}
	if line == "" {
		line = "Nuevo hilo"
	}
	return line
}

// provisionalBranch gives T3 a distinct initial branch for each item.
func provisionalBranch(item service.Item) string {
	return "dory/" + item.ID[:min(8, len(item.ID))]
}

// itemWorktree only detects reuse; T3 chooses the path for new worktrees.
func itemWorktree(item service.Item) (string, bool) {
	for _, ref := range item.Refs {
		if strings.HasPrefix(ref, worktreeRefPrefix) && strings.TrimPrefix(ref, worktreeRefPrefix) != "" {
			return strings.TrimPrefix(ref, worktreeRefPrefix), false
		}
	}
	return "", true
}

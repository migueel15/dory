package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// reviewThread finds the implementation thread of a pull request: the newest
// active one among the items whose refs hold the pull request URL.
func reviewThread(r *http.Request, svc service.Service, prURL string) (*service.ItemThread, error) {
	items, err := svc.Items().List(r.Context(), service.Filter{Fields: map[string]any{"refs": []string{prURL}}, Limit: 1000})
	if err != nil {
		return nil, err
	}
	var best *service.ItemThread
	for _, item := range items {
		links, err := svc.ItemThreads().List(r.Context(), service.Filter{Fields: map[string]any{"item_id": item.ID, "role": "implementation"}, Limit: 1000})
		if err != nil {
			return nil, err
		}
		for i := range links {
			if best == nil || links[i].CreatedAt.After(best.CreatedAt) {
				best = &links[i]
			}
		}
	}
	return best, nil
}

// reviewPrompt asks about a fragment of the pull request without changing code.
func reviewPrompt(number int, path string, start, end int, snippet, question string) string {
	lines := strconv.Itoa(start)
	if end != start {
		lines += "–" + strconv.Itoa(end)
	}
	return fmt.Sprintf("Consulta de revisión sobre PR #%d, fichero %s líneas %s:\n```diff\n%s\n```\n%s\n\nResponde sin modificar código salvo que se pida.", number, path, lines, strings.TrimRight(snippet, "\n"), question)
}

type reviewAgent struct {
	Available     bool   `json:"available"`
	ItemID        string `json:"item_id,omitempty"`
	ThreadID      string `json:"thread_id,omitempty"`
	URL           string `json:"url,omitempty"`
	EnvironmentID string `json:"t3_environment_id,omitempty"`
}

func agentOf(monitor *t3.Monitor, link *service.ItemThread) reviewAgent {
	if link == nil {
		return reviewAgent{}
	}
	out := reviewAgent{Available: true, ItemID: link.ItemID, ThreadID: link.ThreadID}
	if th, ok := monitor.Thread(link.ThreadID); ok {
		out.URL, out.EnvironmentID = th.URL, th.EnvironmentID
	}
	return out
}

func mountReviewAsk(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor) {
	prURL := func(r *http.Request) (string, int, bool) {
		number, err := strconv.Atoi(r.PathValue("number"))
		return fmt.Sprintf("https://github.com/%s/%s/pull/%d", r.PathValue("owner"), r.PathValue("repo"), number), number, err == nil && number > 0
	}
	mux.HandleFunc("GET /api/github/repos/{owner}/{repo}/pulls/{number}/agent", func(w http.ResponseWriter, r *http.Request) {
		url, _, ok := prURL(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid number")
			return
		}
		link, err := reviewThread(r, svc, url)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, agentOf(monitor, link))
	})
	mux.HandleFunc("POST /api/github/repos/{owner}/{repo}/pulls/{number}/ask", func(w http.ResponseWriter, r *http.Request) {
		url, number, ok := prURL(r)
		var body struct {
			Path     string `json:"path"`
			Start    int    `json:"start"`
			End      int    `json:"end"`
			Snippet  string `json:"snippet"`
			Question string `json:"question"`
		}
		if !ok || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body) != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid body")
			return
		}
		body.Question = strings.TrimSpace(body.Question)
		if body.Path == "" || body.Question == "" || body.Start <= 0 || body.End < body.Start {
			writeError(w, http.StatusBadRequest, "invalid_argument", "path, start, end and question are required")
			return
		}
		link, err := reviewThread(r, svc, url)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if link == nil {
			writeError(w, http.StatusConflict, "conflict", "the pull request has no Dory item with an implementation thread")
			return
		}
		text := reviewPrompt(number, body.Path, body.Start, body.End, body.Snippet, body.Question)
		c, err := refineClient(r, monitor, *link)
		if err == nil {
			err = c.SendMessage(r.Context(), link.ThreadID, text)
		}
		if err != nil {
			writeT3Error(w, err)
			return
		}
		monitor.Refresh(link.InstanceID)
		audit(r, svc, link.ThreadID, map[string]any{"operation": "t3.message", "item_id": link.ItemID, "text": text})
		writeJSON(w, http.StatusOK, agentOf(monitor, link))
	})
}

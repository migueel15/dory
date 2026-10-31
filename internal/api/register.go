// Package api owns REST routes. The REST slice implements Register here.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// actor identifies the writer in audit events using the authenticated identity.
func actor(r *http.Request) string { return auth.Actor(r.Context()) }

// Register mounts /api routes; auth owns /api/auth/* on the same mux.
func Register(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor, gh *github.Client) {
	mountResource(mux, "projects", svc.Projects(), func(v *service.Project) *service.Record { return &v.Record })
	mountResource(mux, "repositories", svc.Repositories(), func(v *service.Repository) *service.Record { return &v.Record })
	mountResource(mux, "environments", svc.Environments(), func(v *service.Environment) *service.Record { return &v.Record })
	mountResource(mux, "repository-locations", svc.RepositoryLocations(), func(v *service.RepositoryLocation) *service.Record { return &v.Record })
	mountResource(mux, "t3-instances", svc.T3Instances(), func(v *service.T3Instance) *service.Record { return &v.Record })
	mountResource(mux, "item-threads", svc.ItemThreads(), func(v *service.ItemThread) *service.Record { return &v.Record })
	mountResource(mux, "features", svc.Features(), func(v *service.Feature) *service.Record { return &v.Record })
	mountResource(mux, "items", svc.Items(), func(v *service.Item) *service.Record { return &v.Record })
	mountResource(mux, "memories", svc.Memories(), func(v *service.Memory) *service.Record { return &v.Record })
	mountResource(mux, "links", svc.Links(), func(v *service.Link) *service.Record { return &v.Record })
	mux.HandleFunc("GET /api/t3/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, monitor.Statuses())
	})
	mux.HandleFunc("GET /api/t3/usage", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, monitor.Usage())
	})
	mux.HandleFunc("POST /api/t3/instances/{id}/pair", func(w http.ResponseWriter, r *http.Request) {
		link, err := monitor.PairingLink(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadGateway, "upstream", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": link})
	})
	mux.HandleFunc("GET /api/github/open", func(w http.ResponseWriter, r *http.Request) {
		overview, err := gh.Open(r.Context(), r.URL.Query().Get("refresh") == "true")
		if err != nil {
			writeGithubError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, overview)
	})
	mux.HandleFunc("GET /api/github/repos/{owner}/{repo}/{kind}/{number}", func(w http.ResponseWriter, r *http.Request) {
		number, err := strconv.Atoi(r.PathValue("number"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid number")
			return
		}
		detail, err := gh.Detail(r.Context(), r.PathValue("owner")+"/"+r.PathValue("repo"), r.PathValue("kind"), number)
		if err != nil {
			writeGithubError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	})
	mux.HandleFunc("POST /api/github/repos/{owner}/{repo}/{kind}/{number}/actions", func(w http.ResponseWriter, r *http.Request) {
		number, err := strconv.Atoi(r.PathValue("number"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid number")
			return
		}
		var action github.Action
		if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid action")
			return
		}
		if err := gh.Act(r.Context(), r.PathValue("owner")+"/"+r.PathValue("repo"), r.PathValue("kind"), number, action); err != nil {
			writeGithubError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/github/repos/{owner}/{repo}/pulls/{number}/diff", func(w http.ResponseWriter, r *http.Request) {
		number, err := strconv.Atoi(r.PathValue("number"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid number")
			return
		}
		diff, err := gh.Diff(r.Context(), r.PathValue("owner")+"/"+r.PathValue("repo"), number)
		if err != nil {
			writeGithubError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"diff": diff})
	})
	mux.HandleFunc("GET /api/search", search(svc))
	mux.HandleFunc("GET /api/events/stream", streamEvents(svc))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
	})
}

func writeGithubError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, github.ErrUnconfigured):
		writeError(w, http.StatusServiceUnavailable, "unavailable", err.Error())
	case errors.Is(err, github.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case errors.Is(err, github.ErrUnknownRepo):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	default:
		writeError(w, http.StatusBadGateway, "upstream", err.Error())
	}
}

// Package api owns REST routes. The REST slice implements Register here.
package api

import (
	"net/http"

	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// actor identifies the writer in audit events using the authenticated identity.
func actor(r *http.Request) string { return auth.Actor(r.Context()) }

// Register mounts /api routes; auth owns /api/auth/* on the same mux.
func Register(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor) {
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
	mux.HandleFunc("GET /api/search", search(svc))
	mux.HandleFunc("GET /api/events/stream", streamEvents(svc))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
	})
}

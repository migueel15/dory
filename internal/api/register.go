// Package api owns REST routes. The REST slice implements Register here.
package api

import (
	"net/http"

	"github.com/migueel15/dory/internal/service"
)

// actor identifies the writer in audit events until the auth slice supplies one.
func actor(*http.Request) string { return "web" }

// Register mounts /api routes; auth owns /api/auth/* on the same mux.
func Register(mux *http.ServeMux, svc service.Service) {
	mountResource(mux, "projects", svc.Projects(), func(v *service.Project) *service.Record { return &v.Record })
	mountResource(mux, "repositories", svc.Repositories(), func(v *service.Repository) *service.Record { return &v.Record })
	mountResource(mux, "environments", svc.Environments(), func(v *service.Environment) *service.Record { return &v.Record })
	mountResource(mux, "repository-locations", svc.RepositoryLocations(), func(v *service.RepositoryLocation) *service.Record { return &v.Record })
	mountResource(mux, "features", svc.Features(), func(v *service.Feature) *service.Record { return &v.Record })
	mountResource(mux, "items", svc.Items(), func(v *service.Item) *service.Record { return &v.Record })
	mountResource(mux, "memories", svc.Memories(), func(v *service.Memory) *service.Record { return &v.Record })
	mountResource(mux, "links", svc.Links(), func(v *service.Link) *service.Record { return &v.Record })
	mux.HandleFunc("GET /api/search", search(svc))
	mux.HandleFunc("GET /api/events/stream", streamEvents(svc))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
	})
}

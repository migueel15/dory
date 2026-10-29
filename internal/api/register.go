// Package api owns REST routes. The REST slice implements Register here.
package api

import (
	"github.com/migueel15/dory/internal/service"
	"net/http"
)

// Register mounts /api routes; auth owns /api/auth/* on the same mux.
func Register(mux *http.ServeMux, svc service.Service) {
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "REST implementation pending", http.StatusNotImplemented)
	})
}

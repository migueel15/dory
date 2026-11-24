package api

import (
	"github.com/migueel15/dory/internal/apperr"
	"net/http"

	"github.com/migueel15/dory/internal/github"
)

// mountGitHubAuth links this node to a GitHub account with the device flow.
// The token stays on this node and is never returned.
func (s *Server) mountGitHubAuth(mux *http.ServeMux) {
	gh := s.GitHub
	mux.HandleFunc("GET /api/github/auth", func(w http.ResponseWriter, r *http.Request) {
		source, login := gh.Status(r.Context())
		writeJSON(w, http.StatusOK, map[string]string{"source": source, "login": login})
	})
	mux.HandleFunc("DELETE /api/github/auth", func(w http.ResponseWriter, r *http.Request) {
		if err := gh.Auth.Logout(r.Context()); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/github/auth/device", func(w http.ResponseWriter, r *http.Request) {
		code, err := gh.Auth.StartDevice(r.Context())
		if err != nil {
			writeErr(w, r, apperr.New(apperr.Upstream, "could not reach GitHub to start the login"))
			return
		}
		writeJSON(w, http.StatusOK, code)
	})
	mux.HandleFunc("GET /api/github/auth/device", func(w http.ResponseWriter, r *http.Request) {
		status := gh.Auth.Device()
		if status == nil {
			status = &github.DeviceStatus{State: github.DeviceExpired}
		}
		writeJSON(w, http.StatusOK, status)
	})
}

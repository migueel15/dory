package api

import (
	"github.com/migueel15/dory/internal/apperr"
	"net/http"
	"strings"
)

// mountProviders manages the harnesses of any node of the mesh (requirements
// RF-10, RF-11): calls for another node are forwarded to it, so credentials
// never leave the machine they belong to.
func (s *Server) mountProviders(mux *http.ServeMux) {
	here := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			node := r.PathValue("node")
			if node == "local" || node == s.Node.ID {
				h(w, r)
				return
			}
			if s.Remote == nil || !s.Remote.Forward(w, r, node) {
				writeErr(w, r, apperr.New(apperr.NodeOffline, "the node is not connected"))
			}
		}
	}
	mux.HandleFunc("PUT /api/nodes/{node}/icon", here(s.setIcon))
	mux.HandleFunc("GET /api/nodes/{node}/providers", here(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Runtime.Providers(r.Context()))
	}))
	mux.HandleFunc("PUT /api/nodes/{node}/providers/{id}", here(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &body) {
			return
		}
		if err := s.Runtime.SetEnabled(r.Context(), r.PathValue("id"), body.Enabled); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/nodes/{node}/accounts", here(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Driver string `json:"driver"`
			Name   string `json:"name"`
			Home   string `json:"home"`
		}
		if !decode(w, r, &body) {
			return
		}
		a, err := s.Runtime.AddAccount(r.Context(), body.Driver, body.Name, body.Home)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	mux.HandleFunc("PUT /api/nodes/{node}/accounts/{id}", here(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
			Home string `json:"home"`
		}
		if !decode(w, r, &body) {
			return
		}
		if err := s.Runtime.UpdateAccount(r.Context(), r.PathValue("id"), body.Name, body.Home); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("DELETE /api/nodes/{node}/accounts/{id}", here(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Runtime.RemoveAccount(r.PathValue("id")); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/nodes/{node}/providers/{id}/{kind}", here(func(w http.ResponseWriter, r *http.Request) {
		job, err := s.Runtime.StartJob(r.PathValue("id"), r.PathValue("kind"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, job)
	}))
	mux.HandleFunc("GET /api/nodes/{node}/jobs/{id}", here(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		job, ok := s.Runtime.JobState(r.PathValue("id"))
		if !ok {
			writeErr(w, r, apperr.New(apperr.NotFound, "job not found"))
			return
		}
		writeJSON(w, http.StatusOK, job)
	}))
	mux.HandleFunc("POST /api/nodes/{node}/jobs/{id}/code", here(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Code string `json:"code"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if !decode(w, r, &body) {
			return
		}
		code := strings.TrimSpace(body.Code)
		if code == "" || len(code) > 2048 || strings.ContainsAny(code, "\r\n\x00") {
			writeErr(w, r, apperr.New(apperr.InvalidCode, "paste the authorization code in a single line"))
			return
		}
		err := s.Runtime.SubmitLoginCode(r.PathValue("id"), code)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

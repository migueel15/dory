package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/runtime"
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
				writeError(w, http.StatusServiceUnavailable, "node_offline", "el nodo no está conectado")
			}
		}
	}
	mux.HandleFunc("GET /api/nodes/{node}/providers", here(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Runtime.Providers(r.Context()))
	}))
	mux.HandleFunc("POST /api/nodes/{node}/providers/{id}/{kind}", here(func(w http.ResponseWriter, r *http.Request) {
		job, err := s.Runtime.StartJob(r.PathValue("id"), r.PathValue("kind"))
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, job)
	}))
	mux.HandleFunc("GET /api/nodes/{node}/jobs/{id}", here(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		job, ok := s.Runtime.JobState(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "job not found")
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
			writeError(w, http.StatusBadRequest, "invalid_code", "pega el código de autorización en una sola línea")
			return
		}
		err := s.Runtime.SubmitLoginCode(r.PathValue("id"), code)
		if errors.Is(err, runtime.ErrJobInputClosed) {
			writeError(w, http.StatusConflict, "job_input_closed", err.Error())
			return
		}
		if err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

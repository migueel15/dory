package api

import "net/http"

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
		job, ok := s.Runtime.JobState(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "job not found")
			return
		}
		writeJSON(w, http.StatusOK, job)
	}))
}

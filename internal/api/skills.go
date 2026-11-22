package api

import (
	"net/http"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/skills"
)

// mountSkills serves the skills catalog and the state of each skill on each
// node (requirements RF-37 to RF-41).
func (s *Server) mountSkills(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/skills", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.Store.Skills().List(r.Context(), data.Filter{Limit: 1000})
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})
	mux.HandleFunc("GET /api/skills/states", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.Store.SkillStates().List(r.Context(), data.Filter{Limit: 1000})
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})
	save := func(w http.ResponseWriter, r *http.Request, id string) {
		var sk data.Skill
		if !decode(w, r, &sk) {
			return
		}
		sk.ID = id
		out, err := skills.Save(r.Context(), s.Store, sk)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
	mux.HandleFunc("POST /api/skills", func(w http.ResponseWriter, r *http.Request) { save(w, r, "") })
	mux.HandleFunc("PUT /api/skills/{id}", func(w http.ResponseWriter, r *http.Request) { save(w, r, r.PathValue("id")) })
	mux.HandleFunc("PUT /api/skills/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &body) {
			return
		}
		out, err := skills.SetEnabled(r.Context(), s.Store, r.PathValue("id"), body.Enabled)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("POST /api/skills/{id}/fork", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &body) {
			return
		}
		out, err := skills.Fork(r.Context(), s.Store, r.PathValue("id"), body.Name)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	// Conflicts live in the folders of one node, so these routes run there.
	mux.HandleFunc("GET /api/nodes/{node}/skills/{id}/conflicts", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		if s.Skills == nil {
			writeError(w, http.StatusNotImplemented, "unavailable", "este nodo no instala skills")
			return
		}
		out, err := s.Skills.Conflicts(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	mux.HandleFunc("POST /api/nodes/{node}/skills/{id}/resolve", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action  string `json:"action"`
			Harness string `json:"harness"`
		}
		if !decode(w, r, &body) {
			return
		}
		if s.Skills == nil {
			writeError(w, http.StatusNotImplemented, "unavailable", "este nodo no instala skills")
			return
		}
		backup, err := s.Skills.Resolve(r.Context(), r.PathValue("id"), body.Action, body.Harness)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"backup": backup})
	}))
	mux.HandleFunc("DELETE /api/skills/{id}", func(w http.ResponseWriter, r *http.Request) {
		if sk, err := s.Store.Skills().Get(r.Context(), r.PathValue("id")); err == nil && sk.Source == "system" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "las skills del sistema no se pueden archivar; desactívala")
			return
		}
		if err := s.Store.Skills().Archive(r.Context(), r.PathValue("id")); err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

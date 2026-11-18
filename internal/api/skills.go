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
	mux.HandleFunc("DELETE /api/skills/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Skills().Archive(r.Context(), r.PathValue("id")); err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

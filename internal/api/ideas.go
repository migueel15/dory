package api

import (
	"net/http"

	"github.com/migueel15/dory/internal/runtime"
)

func (s *Server) mountIdeas(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/ideas", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID          string               `json:"id"`
			Text        string               `json:"text"`
			ProjectID   *string              `json:"project_id"`
			Attachments []runtime.Attachment `json:"attachments"`
		}
		if !decode(w, r, &body) {
			return
		}
		t, err := s.Ideas.Create(r.Context(), body.ID, body.Text, body.ProjectID, body.Attachments)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, t)
	})
	mux.HandleFunc("GET /api/title-model", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Runtime.TitleModel())
	})
	mux.HandleFunc("PUT /api/title-model", func(w http.ResponseWriter, r *http.Request) {
		var body runtime.TitleModel
		if !decode(w, r, &body) {
			return
		}
		if err := s.Runtime.SetTitleModel(body); err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, body)
	})
}

package api

import (
	"encoding/json"
	"net/http"

	"github.com/migueel15/dory/internal/service"
)

// mountPlans exposes the versioned plan of an item. Agents propose versions
// through MCP; here the user edits a version or accepts the newest one.
func mountPlans(mux *http.ServeMux, svc service.Service) {
	mux.HandleFunc("GET /api/items/{id}/plans", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := svc.Items().Get(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		plans, err := svc.ItemPlans().List(r.Context(), service.Filter{Fields: map[string]any{"item_id": id}, Limit: 1000})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, plans)
	})
	mux.HandleFunc("POST /api/items/{id}/plans", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Sections      service.PlanSections `json:"sections"`
			OpenQuestions []string             `json:"open_questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid body")
			return
		}
		plan, err := service.ProposePlan(r.Context(), svc, actor(r), r.PathValue("id"), "user", body.Sections, body.OpenQuestions)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, plan)
	})
	mux.HandleFunc("POST /api/items/{id}/plan/accept", func(w http.ResponseWriter, r *http.Request) {
		item, plan, err := service.AcceptPlan(r.Context(), svc, actor(r), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item, "plan": plan})
	})
}

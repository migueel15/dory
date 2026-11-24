package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/data"
)

func TestTaskResourceWrites(t *testing.T) {
	s, task := newPlanServer(t)
	mux := http.NewServeMux()
	mountResource(mux, "tasks", s.Store.Tasks(), resourceWrites[data.Task]{s.CreateTask, s.PatchTask})
	req := func(method, path, body, match string, want int) {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("If-Match", match)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d, want %d (%s)", method, path, w.Code, want, w.Body)
		}
	}
	path := "/api/tasks/" + task.ID
	req("POST", "/api/tasks", `{"id":"`+task.ID+`","title":"Duplicada"}`, "", 409)
	req("PATCH", path, `{"description":"Editar","issue_number":123}`, `"`+task.UpdatedAt+`"`, 200)
	req("PUT", path, `{"title":"Antigua","updated_at":"`+task.UpdatedAt+`"}`, "", 409)
	req("PUT", path, `{"issue_number":null}`, "", 200)
	req("PATCH", path, `{"unknown":true}`, "", 400)
	req("PATCH", "/api/tasks/missing", `{"title":"Ausente"}`, "", 404)
	got, err := s.Store.Tasks().Get(t.Context(), task.ID)
	if err != nil || got.Description != "Editar" || got.Title != task.Title || got.IssueNumber != nil {
		t.Fatalf("task = %+v, err = %v", got, err)
	}
}

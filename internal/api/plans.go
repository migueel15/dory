package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/migueel15/dory/internal/data"
)

// ErrPlanConflict refuses a save based on an older revision of the plan.
var ErrPlanConflict = errors.New("the plan changed since it was read; read it again")

// planMu serializes the saves of plans on this node, so a read-modify-write
// of the agent and one of the user cannot interleave.
var planMu sync.Mutex

// legacySections are the fixed sections of the plans written before the plan
// became one document, in the order they are shown.
var legacySections = [][2]string{{"goal", "Objetivo"}, {"scope", "Alcance"}, {"where", "Dónde"}, {"design", "Diseño"}, {"requirements", "Requisitos"}, {"validation", "Validación"}, {"risks", "Riesgos"}}

// planMarkdown is the document of a plan; plans from before the document
// are rendered from their sections and open questions.
func planMarkdown(p data.Plan) string {
	if p.BodyMD != "" {
		return p.BodyMD
	}
	sections := map[string]any{}
	json.Unmarshal(p.Sections, &sections)
	var b strings.Builder
	write := func(title string, v any) {
		if text := strings.TrimSpace(fmt.Sprint(v)); v != nil && text != "" {
			fmt.Fprintf(&b, "## %s\n\n%s\n\n", title, text)
		}
	}
	for _, s := range legacySections {
		write(s[1], sections[s[0]])
		delete(sections, s[0])
	}
	for k, v := range sections {
		write(k, v)
	}
	if len(p.OpenQuestions) > 0 {
		b.WriteString("## Preguntas abiertas\n\n")
		for _, q := range p.OpenQuestions {
			fmt.Fprintf(&b, "- %s\n", q)
		}
	}
	return strings.TrimSpace(b.String())
}

// latestPlan is the newest version of a task's plan; ok is false without one.
func (s *Server) latestPlan(ctx context.Context, taskID string) (p data.Plan, ok bool, err error) {
	plans, err := s.Store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": taskID}, Limit: 1000})
	if err != nil {
		return p, false, err
	}
	for _, x := range plans {
		if !ok || x.Version > p.Version {
			p, ok = x, true
		}
	}
	return p, ok, nil
}

// PlanDocument is the current plan of a task with its document filled in. A
// task without a plan gives an empty document at revision 0.
func (s *Server) PlanDocument(ctx context.Context, taskID string) (data.Plan, error) {
	if _, err := s.Store.Tasks().Get(ctx, taskID); err != nil {
		return data.Plan{}, err
	}
	p, ok, err := s.latestPlan(ctx, taskID)
	if err != nil || !ok {
		return data.Plan{TaskID: taskID}, err
	}
	p.BodyMD = planMarkdown(p)
	return p, nil
}

// WritePlan replaces the plan document. With base >= 0 it refuses the save
// when the plan is no longer at that revision. Changing an accepted plan
// leaves it waiting for the user to accept it again.
func (s *Server) WritePlan(ctx context.Context, taskID, body string, base int64, author string) (data.Plan, error) {
	planMu.Lock()
	defer planMu.Unlock()
	return s.writePlan(ctx, taskID, body, base, author)
}

func (s *Server) writePlan(ctx context.Context, taskID, body string, base int64, author string) (data.Plan, error) {
	cur, err := s.PlanDocument(ctx, taskID)
	if err != nil {
		return cur, err
	}
	if base >= 0 && base != cur.Revision {
		return cur, fmt.Errorf("%w (now at revision %d)", ErrPlanConflict, cur.Revision)
	}
	body = strings.TrimSpace(body)
	if cur.ID != "" && body == cur.BodyMD {
		return cur, nil
	}
	if cur.ID == "" {
		// Archived versions keep their numbers, so the new one goes after them.
		all, err := s.Store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": taskID}, Limit: 1000, IncludeArchived: true})
		if err != nil {
			return cur, err
		}
		cur.Version = 1
		for _, x := range all {
			cur.Version = max(cur.Version, x.Version+1)
		}
	}
	cur.BodyMD, cur.Revision, cur.Author, cur.AcceptedAt = body, cur.Revision+1, author, nil
	return s.Store.Plans().Put(ctx, cur)
}

// EditPlan replaces one exact piece of the plan document. The piece must
// appear once, so an edit never lands in the wrong place.
func (s *Server) EditPlan(ctx context.Context, taskID, old, replacement string, base int64, author string) (data.Plan, error) {
	planMu.Lock()
	defer planMu.Unlock()
	cur, err := s.PlanDocument(ctx, taskID)
	if err != nil {
		return cur, err
	}
	if base >= 0 && base != cur.Revision {
		return cur, fmt.Errorf("%w (now at revision %d)", ErrPlanConflict, cur.Revision)
	}
	if old == "" {
		return cur, fmt.Errorf("%w: old_text is empty; use plan_write to write the whole plan", data.ErrInvalid)
	}
	switch n := strings.Count(cur.BodyMD, old); n {
	case 0:
		return cur, fmt.Errorf("%w: old_text is not in the plan; read it again", data.ErrInvalid)
	case 1:
	default:
		return cur, fmt.Errorf("%w: old_text appears %d times; include more context", data.ErrInvalid, n)
	}
	return s.writePlan(ctx, taskID, strings.Replace(cur.BodyMD, old, replacement, 1), cur.Revision, author)
}

func (s *Server) planRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.PlanDocument(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
	mux.HandleFunc("PUT /api/tasks/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			BodyMD   string `json:"body_md"`
			Revision int64  `json:"revision"`
		}
		if !decode(w, r, &body) {
			return
		}
		p, err := s.WritePlan(r.Context(), r.PathValue("id"), body.BodyMD, body.Revision, "user")
		if errors.Is(err, ErrPlanConflict) {
			writeError(w, http.StatusConflict, "plan_conflict", err.Error())
			return
		}
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
}

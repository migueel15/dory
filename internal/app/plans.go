package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
)

// ErrPlanConflict refuses a save based on an older revision of the plan.
var ErrPlanConflict = apperr.New(apperr.RevisionConflict, "the plan changed since it was read; read it again")

// Plans reads and writes the plan document of a task. Its lock serializes the
// saves on this node, so a read-modify-write of the agent and one of the user
// cannot interleave.
type Plans struct {
	Store *data.Store

	mu sync.Mutex
}

// legacySections are the fixed sections of the plans written before the plan
// became one document, in the order they are shown.
var legacySections = [][2]string{{"goal", "Objetivo"}, {"scope", "Alcance"}, {"where", "Dónde"}, {"design", "Diseño"}, {"requirements", "Requisitos"}, {"validation", "Validación"}, {"risks", "Riesgos"}}

// PlanMarkdown is the document of a plan; plans from before the document
// are rendered from their sections and open questions.
func PlanMarkdown(p data.Plan) string {
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

// latest is the newest version of a task's plan; ok is false without one.
func (s *Plans) latest(ctx context.Context, taskID string) (p data.Plan, ok bool, err error) {
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

// Document is the current plan of a task with its document filled in. A
// task without a plan gives an empty document at revision 0.
func (s *Plans) Document(ctx context.Context, taskID string) (data.Plan, error) {
	if _, err := s.Store.Tasks().Get(ctx, taskID); err != nil {
		return data.Plan{}, err
	}
	p, ok, err := s.latest(ctx, taskID)
	if err != nil || !ok {
		return data.Plan{TaskID: taskID}, err
	}
	p.BodyMD = PlanMarkdown(p)
	return p, nil
}

// Write replaces the plan document. With base >= 0 it refuses the save
// when the plan is no longer at that revision. Changing an accepted plan
// leaves it waiting for the user to accept it again.
func (s *Plans) Write(ctx context.Context, taskID, body string, base int64, author string) (data.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(ctx, taskID, body, base, author)
}

func (s *Plans) write(ctx context.Context, taskID, body string, base int64, author string) (data.Plan, error) {
	cur, err := s.Document(ctx, taskID)
	if err != nil {
		return cur, err
	}
	if base >= 0 && base != cur.Revision {
		return cur, ErrPlanConflict.Msgf("now at revision %d", cur.Revision).With("revision", cur.Revision)
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
	if cur.ID == "" {
		return s.Store.Plans().Create(ctx, cur)
	}
	out, err := s.Store.Plans().PatchIf(ctx, cur.ID, cur.UpdatedAt, data.Fields{
		"body_md": body, "revision": cur.Revision, "author": author, "accepted_at": nil,
	})
	if errors.Is(err, data.ErrRevisionConflict) {
		return cur, ErrPlanConflict
	}
	return out, err
}

// Edit replaces one exact piece of the plan document. The piece must
// appear once, so an edit never lands in the wrong place.
func (s *Plans) Edit(ctx context.Context, taskID, old, replacement string, base int64, author string) (data.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.Document(ctx, taskID)
	if err != nil {
		return cur, err
	}
	if base >= 0 && base != cur.Revision {
		return cur, ErrPlanConflict.Msgf("now at revision %d", cur.Revision).With("revision", cur.Revision)
	}
	if old == "" {
		return cur, data.ErrInvalid.Msgf("old_text is empty; use plan_write to write the whole plan")
	}
	switch n := strings.Count(cur.BodyMD, old); n {
	case 0:
		return cur, data.ErrInvalid.Msgf("old_text is not in the plan; read it again")
	case 1:
	default:
		return cur, data.ErrInvalid.Msgf("old_text appears %d times; include more context", n)
	}
	return s.write(ctx, taskID, strings.Replace(cur.BodyMD, old, replacement, 1), cur.Revision, author)
}

// Pending says whether the latest plan of a task waits for the user to accept
// it. A task without a plan is not pending.
func (s *Plans) Pending(ctx context.Context, taskID string) (bool, error) {
	latest, ok, err := s.latest(ctx, taskID)
	if err != nil || !ok {
		return false, err
	}
	return latest.AcceptedAt == nil, nil
}

// Accept marks the latest plan accepted and the task ready.
func (s *Plans) Accept(ctx context.Context, taskID string) (data.Task, data.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	latest, ok, err := s.latest(ctx, taskID)
	if err != nil {
		return data.Task{}, data.Plan{}, err
	}
	if !ok {
		return data.Task{}, data.Plan{}, data.ErrInvalid.Msgf("the task has no plan")
	}
	return s.Store.AcceptPlan(ctx, latest)
}

// List is every version of the plan of a task.
func (s *Plans) List(ctx context.Context, taskID string) ([]data.Plan, error) {
	return s.Store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": taskID}, Limit: 1000})
}

// WriteByAgent saves a whole plan for an agent. Rewriting an existing plan
// blind would drop what the user is writing, so it needs the revision the
// agent read; a nil revision only creates the first version.
func (s *Plans) WriteByAgent(ctx context.Context, taskID, body string, revision *int64) (data.Plan, error) {
	base := int64(-1)
	if revision != nil {
		base = *revision
	} else if cur, err := s.Document(ctx, taskID); err != nil {
		return cur, err
	} else if cur.Revision > 0 {
		return cur, apperr.New(apperr.Conflict, "the plan already exists: change parts of it with plan_edit, or pass the revision from plan_read to rewrite it")
	}
	return s.Write(ctx, taskID, body, base, "agent")
}

// EditByAgent replaces one exact piece of the plan for an agent; a nil
// revision skips the revision check.
func (s *Plans) EditByAgent(ctx context.Context, taskID, old, replacement string, revision *int64) (data.Plan, error) {
	base := int64(-1)
	if revision != nil {
		base = *revision
	}
	return s.Edit(ctx, taskID, old, replacement, base, "agent")
}

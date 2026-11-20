package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/notify"
	"github.com/migueel15/dory/internal/runtime"
)

// Card is a task as the kanban shows it: its column is derived from its
// threads and its pull request, never stored.
type Card struct {
	data.Task
	Column   string        `json:"column"`
	Repo     string        `json:"repo,omitempty"`
	Threads  []data.Thread `json:"threads"`
	Pull     *github.Pull  `json:"pull,omitempty"`
	PRState  string        `json:"pr_state,omitempty"`
	Blockers []string      `json:"blockers"`
	HasPlan  bool          `json:"has_plan"`
}

// Columns of the kanban, in order (requirements 5.11).
var Columns = []string{"inbox", "ready", "working", "needs_you", "in_review", "ready_to_merge", "done", "cancelled"}

func (s *Server) mountTasks(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/board", s.board)
	mux.HandleFunc("GET /api/tasks/{id}/card", func(w http.ResponseWriter, r *http.Request) {
		t, err := s.Store.Tasks().Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDataError(w, err)
			return
		}
		cards, err := s.Cards(r.Context(), []data.Task{t})
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, cards[0])
	})
	mux.HandleFunc("GET /api/tasks/{id}/dependencies", func(w http.ResponseWriter, r *http.Request) {
		deps, err := s.Store.Dependencies().List(r.Context(), data.Filter{Fields: map[string]any{"task_id": r.PathValue("id")}, Limit: 1000})
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, deps)
	})
	mux.HandleFunc("PUT /api/tasks/{id}/dependencies/{dep}", func(w http.ResponseWriter, r *http.Request) {
		d, err := s.Store.Dependencies().Put(r.Context(), data.Dependency{TaskID: r.PathValue("id"), DependsOnID: r.PathValue("dep")})
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	})
	mux.HandleFunc("DELETE /api/tasks/{id}/dependencies/{dep}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Dependencies().Archive(r.Context(), r.PathValue("id")+"/"+r.PathValue("dep")); err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.planRoutes(mux)
	mux.HandleFunc("GET /api/tasks/{id}/plans", func(w http.ResponseWriter, r *http.Request) {
		plans, err := s.Store.Plans().List(r.Context(), data.Filter{Fields: map[string]any{"task_id": r.PathValue("id")}, Limit: 1000})
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, plans)
	})
	mux.HandleFunc("POST /api/tasks/{id}/plan/accept", func(w http.ResponseWriter, r *http.Request) {
		t, p, err := s.acceptPlan(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"task": t, "plan": p})
	})
	mux.HandleFunc("POST /api/tasks/{id}/launch", s.launch)
}

// planPending says whether the latest plan of a task waits for the user to
// accept it. A task without a plan is not pending.
func (s *Server) planPending(ctx context.Context, taskID string) (bool, error) {
	plans, err := s.Store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": taskID}, Limit: 1000})
	if err != nil || len(plans) == 0 {
		return false, err
	}
	latest := plans[0]
	for _, p := range plans {
		if p.Version > latest.Version {
			latest = p
		}
	}
	return latest.AcceptedAt == nil, nil
}

// acceptPlan marks the latest plan accepted and the task ready.
func (s *Server) acceptPlan(ctx context.Context, taskID string) (data.Task, data.Plan, error) {
	plans, err := s.Store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": taskID}, Limit: 1000})
	if err != nil {
		return data.Task{}, data.Plan{}, err
	}
	if len(plans) == 0 {
		return data.Task{}, data.Plan{}, fmt.Errorf("%w: the task has no plan", data.ErrInvalid)
	}
	latest := plans[0]
	for _, p := range plans {
		if p.Version > latest.Version {
			latest = p
		}
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	latest.AcceptedAt = &at
	if latest, err = s.Store.Plans().Put(ctx, latest); err != nil {
		return data.Task{}, data.Plan{}, err
	}
	t, err := s.Store.Tasks().Get(ctx, taskID)
	if err != nil {
		return t, latest, err
	}
	if t.Stage == "inbox" {
		t.Stage = "ready"
		t, err = s.Store.Tasks().Put(ctx, t)
	}
	return t, latest, err
}

func (s *Server) board(w http.ResponseWriter, r *http.Request) {
	f := data.Filter{Limit: 1000}
	if p := r.URL.Query().Get("project"); p != "" {
		f.Fields = map[string]any{"project_id": p}
	}
	tasks, err := s.Store.Tasks().List(r.Context(), f)
	if err != nil {
		writeDataError(w, err)
		return
	}
	cards, err := s.Cards(r.Context(), tasks)
	if err != nil {
		writeDataError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": Columns, "cards": cards})
}

// Cards adds to each task its threads, pull request, blockers and column.
func (s *Server) Cards(ctx context.Context, tasks []data.Task) ([]Card, error) {
	threads, err := s.Store.Threads().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	byTask := map[string][]data.Thread{}
	for _, t := range threads {
		if t.TaskID != nil {
			byTask[*t.TaskID] = append(byTask[*t.TaskID], t)
		}
	}
	repos, err := s.Store.Repositories().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, r := range repos {
		names[r.ID], _ = github.ParseRemote(r.RemoteURL)
	}
	open := s.openPulls(ctx)
	plans, err := s.Store.Plans().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	withPlan := map[string]bool{}
	for _, p := range plans {
		withPlan[p.TaskID] = true
	}
	// Each card may ask GitHub for the final state of its pull request, so
	// the cards are built concurrently and keep the order of tasks.
	cards := make([]Card, len(tasks))
	errs := make([]error, len(tasks))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			cards[i], errs[i] = s.card(ctx, t, byTask, withPlan, names, open)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return cards, nil
}

// openPulls indexes the open pull requests by repository and branch or number.
func (s *Server) openPulls(ctx context.Context) map[string]github.Pull {
	open := map[string]github.Pull{}
	if s.GitHub != nil && s.GitHub.Configured(ctx) {
		if overview, err := s.GitHub.Open(ctx, false); err == nil {
			for _, p := range overview.Pulls {
				open[p.Repo+"#"+p.HeadRef] = p
				open[fmt.Sprintf("%s#%d", p.Repo, p.Number)] = p
			}
		}
	}
	return open
}

func (s *Server) card(ctx context.Context, t data.Task, byTask map[string][]data.Thread, withPlan map[string]bool, names map[string]string, open map[string]github.Pull) (Card, error) {
	c := Card{Task: t, Threads: byTask[t.ID], HasPlan: withPlan[t.ID], Blockers: []string{}}
	if c.Threads == nil {
		c.Threads = []data.Thread{}
	}
	if t.RepositoryID != nil {
		c.Repo = names[*t.RepositoryID]
	}
	s.attachPull(ctx, &c, open)
	blockers, err := s.Store.Blockers(ctx, t.ID)
	if err != nil {
		return c, err
	}
	for _, b := range blockers {
		if !s.finished(ctx, b, open, names) {
			c.Blockers = append(c.Blockers, b.ID)
		}
	}
	c.Column = column(c)
	return c, nil
}

// attachPull finds the pull request of a card: the one of its number, or
// the one whose head is the task branch (requirement RF-21).
func (s *Server) attachPull(ctx context.Context, c *Card, open map[string]github.Pull) {
	if c.Repo == "" {
		return
	}
	var key string
	if c.PRNumber != nil {
		key = fmt.Sprintf("%s#%d", c.Repo, *c.PRNumber)
	} else if c.Branch != "" {
		key = c.Repo + "#" + c.Branch
	} else {
		return
	}
	if p, ok := open[key]; ok {
		c.Pull, c.PRState = &p, "open"
		if p.Draft {
			c.PRState = "draft"
		}
		if c.PRNumber == nil {
			n := int64(p.Number)
			c.PRNumber = &n
		}
		return
	}
	if s.GitHub == nil || !s.GitHub.Configured(ctx) {
		return
	}
	// Not open: ask for its final state.
	ref, err := s.GitHub.PullState(ctx, c.Repo, c.PRNumber, c.Branch)
	if err == nil && ref.Number > 0 {
		n := int64(ref.Number)
		c.PRNumber, c.PRState = &n, ref.State
	}
}

func (s *Server) finished(ctx context.Context, t data.Task, open map[string]github.Pull, names map[string]string) bool {
	c := Card{Task: t}
	if t.RepositoryID != nil {
		c.Repo = names[*t.RepositoryID]
	}
	s.attachPull(ctx, &c, open)
	return c.PRState == "merged" || t.Stage == "cancelled" || doneByHand(c)
}

// doneByHand says whether the user marked the task done and it has no pull
// request. The pull request is the source of truth: with one, the column
// comes from its state and stage is ignored.
func doneByHand(c Card) bool {
	return c.Stage == "done" && c.PRNumber == nil
}

// column applies the rules of requirements 5.11.
func column(c Card) string {
	if c.Stage == "cancelled" || c.PRState == "closed" {
		return "cancelled"
	}
	if c.PRState == "merged" || doneByHand(c) {
		return "done"
	}
	working, waiting := false, false
	for _, t := range c.Threads {
		switch t.Status {
		case "working":
			working = true
		case "waiting", "error":
			waiting = true
		}
	}
	if waiting || c.Pull != nil && (c.Pull.CI == "FAILURE" || c.Pull.CI == "ERROR" || c.Pull.ReviewDecision == "CHANGES_REQUESTED") {
		return "needs_you"
	}
	if working || c.PRState == "draft" {
		return "working"
	}
	if c.Pull != nil {
		if c.Pull.ReviewDecision == "APPROVED" || c.Pull.ReviewDecision == "" && len(c.Pull.ReviewRequests) == 0 {
			return "ready_to_merge"
		}
		return "in_review"
	}
	if len(c.Threads) > 0 {
		return "working"
	}
	if c.Stage == "ready" {
		return "ready"
	}
	return "inbox"
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// branchName follows the user's convention: type/issue-short-description.
func branchName(t data.Task) string {
	words := strings.Fields(slugPattern.ReplaceAllString(strings.ToLower(asciiFold(t.Title)), " "))
	if len(words) > 6 {
		words = words[:6]
	}
	slug := strings.Join(words, "-")
	if slug == "" {
		slug = "tarea"
	}
	if t.IssueNumber != nil {
		return fmt.Sprintf("feat/%d-%s", *t.IssueNumber, slug)
	}
	return fmt.Sprintf("feat/%s-%s", t.ID[:8], slug)
}

func asciiFold(s string) string {
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n").Replace(s)
}

// launch starts an implementation thread for a task on this node, in its own
// worktree and branch.
func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider    string `json:"provider"`
		Model       string `json:"model"`
		Effort      string `json:"effort"`
		RuntimeMode string `json:"runtime_mode"`
		BaseBranch  string `json:"base_branch"`
		Text        string `json:"text"`
	}
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	t, err := s.Store.Tasks().Get(ctx, r.PathValue("id"))
	if err != nil {
		writeDataError(w, err)
		return
	}
	if pending, err := s.planPending(ctx, t.ID); err != nil {
		writeDataError(w, err)
		return
	} else if pending {
		writeError(w, http.StatusConflict, "plan_not_accepted", "acepta el plan de la tarea antes de implementarla")
		return
	}
	var repoPath string
	if t.RepositoryID == nil && t.ProjectID != nil {
		repos, _ := s.Store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": *t.ProjectID}})
		if len(repos) == 1 {
			t.RepositoryID = &repos[0].ID
		}
	}
	if t.RepositoryID != nil {
		repoPath, err = s.Store.RepositoryPath(ctx, *t.RepositoryID)
		if err != nil && !errors.Is(err, data.ErrNotFound) {
			writeDataError(w, err)
			return
		}
	}
	if repoPath == "" {
		writeError(w, http.StatusConflict, "no_repository", "this node has no local path for the task's repository")
		return
	}
	if t.Branch == "" {
		t.Branch = branchName(t)
	}
	if body.BaseBranch == "" {
		repo, _ := s.Store.Repositories().Get(ctx, *t.RepositoryID)
		body.BaseBranch = repo.DefaultBranch
	}
	if t, err = s.Store.Tasks().Put(ctx, t); err != nil {
		writeDataError(w, err)
		return
	}
	th, err := s.Runtime.Create(ctx, runtime.NewThread{ProjectID: t.ProjectID, TaskID: &t.ID, Provider: body.Provider, Model: body.Model,
		Effort: body.Effort, RuntimeMode: body.RuntimeMode, Title: t.Title, Worktree: true, RepoPath: repoPath, Branch: t.Branch, BaseBranch: body.BaseBranch})
	if err != nil {
		writeDataError(w, err)
		return
	}
	prompt := body.Text
	if prompt == "" {
		prompt = s.taskPrompt(ctx, t)
	}
	if err = s.Runtime.Send(ctx, th.ID, prompt); err != nil {
		writeDataError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"thread_id": th.ID, "branch": t.Branch})
}

// taskPrompt is the first message of an implementation thread.
func (s *Server) taskPrompt(ctx context.Context, t data.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Implementa esta tarea de Dory (id %s) en la rama %s.\n\n# %s\n\n%s\n", t.ID, t.Branch, t.Title, t.Description)
	issue := ""
	if t.IssueNumber != nil {
		issue = fmt.Sprint(*t.IssueNumber)
		fmt.Fprintf(&b, "\nIssue de GitHub: #%d.\n", *t.IssueNumber)
	}
	plans, _ := s.Store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": t.ID}})
	var accepted *data.Plan
	for i := range plans {
		if plans[i].AcceptedAt != nil && (accepted == nil || plans[i].Version > accepted.Version) {
			accepted = &plans[i]
		}
	}
	if accepted != nil {
		fmt.Fprintf(&b, "\nPlan aceptado:\n\n%s\n", planMarkdown(*accepted))
	}
	b.WriteString("\nUsa el MCP de Dory para leer el contexto del proyecto y guardar lo que aprendas.\n")
	return autoMessage("implement", map[string]string{"task": t.ID, "title": t.Title, "branch": t.Branch, "issue": issue}, b.String(), "")
}

// TaskColumns gives the kanban column of every open task, with the clock of
// its pull request, for the notices of tasks that reach Ready to merge.
func (s *Server) TaskColumns(ctx context.Context) (map[string]notify.Column, error) {
	tasks, err := s.Store.Tasks().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	cards, err := s.Cards(ctx, tasks)
	if err != nil {
		return nil, err
	}
	out := make(map[string]notify.Column, len(cards))
	for _, c := range cards {
		col := notify.Column{Name: c.Column}
		if c.Pull != nil {
			col.Stamp = fmt.Sprintf("%d@%s", c.Pull.Number, c.Pull.UpdatedAt)
		}
		out[c.ID] = col
	}
	return out, nil
}

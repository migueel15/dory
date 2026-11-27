package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
)

// UnifiedTask is the source-agnostic representation of a task (internal or external).
type UnifiedTask struct {
	ID           string            `json:"id"`
	Source       string            `json:"source"` // internal | github
	ProjectID    *string           `json:"project_id,omitempty"`
	RepositoryID *string           `json:"repository_id,omitempty"`
	Repo         string            `json:"repo,omitempty"`
	Number       *int64            `json:"number,omitempty"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	URL          string            `json:"url,omitempty"`
	State        string            `json:"state,omitempty"`        // open | closed
	StateReason  string            `json:"state_reason,omitempty"` // COMPLETED | NOT_PLANNED | ""
	Stage        string            `json:"stage"`
	Position     string            `json:"position"`
	Author       string            `json:"author,omitempty"`
	Assignees    []string          `json:"assignees,omitempty"`
	Labels       []string          `json:"labels,omitempty"`
	LabelColors  map[string]string `json:"label_colors,omitempty"`
	Milestone    string            `json:"milestone,omitempty"`
	Comments     int               `json:"comments,omitempty"`
	ClosingPRs   []int             `json:"closing_prs,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	ClosedAt       string `json:"closed_at,omitempty"`
	legacyBranch   string
	legacyPRNumber *int64
}

// TaskFilter filters UnifiedTasks across providers.
type TaskFilter struct {
	ProjectID string
	Source    string // "" | internal | github
	Scope     string // "" | all | mine | mine_or_unassigned
	Open      bool
	Viewer    string
}

type TaskCreateInput struct {
	Source       string
	ProjectID    string
	RepositoryID string
	Title        string
	Description  string
	Labels       []string
	Assignees    []string
}

type TaskUpdateInput struct {
	Title       *string
	Description *string
	Labels      *[]string
	Assignees   *[]string
}

type TaskStateInput struct {
	State  string // open | done | cancelled
	Reason string // COMPLETED | NOT_PLANNED | ""
}

// TaskProvider abstracts a task source (internal Dory tasks, GitHub issues, future Linear/Notion).
type TaskProvider interface {
	Source() string
	List(ctx context.Context, filter TaskFilter) ([]UnifiedTask, error)
	Get(ctx context.Context, taskID string) (UnifiedTask, error)
	Create(ctx context.Context, in TaskCreateInput) (UnifiedTask, error)
	Update(ctx context.Context, taskID string, in TaskUpdateInput) (UnifiedTask, error)
	SetState(ctx context.Context, taskID string, in TaskStateInput) (UnifiedTask, error)
	Comment(ctx context.Context, taskID, body string) (github.IssueComment, error)
	ListComments(ctx context.Context, taskID string) ([]github.IssueComment, error)
}

// InternalProvider serves internal Dory tasks from Store.Tasks().
type InternalProvider struct {
	Store *data.Store
}

func (p *InternalProvider) Source() string { return "internal" }

func (p *InternalProvider) List(ctx context.Context, filter TaskFilter) ([]UnifiedTask, error) {
	f := data.Filter{}
	if filter.ProjectID != "" {
		f.Fields = map[string]any{"project_id": filter.ProjectID}
	}
	tasks, err := p.Store.Tasks().ListAll(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]UnifiedTask, 0, len(tasks))
	for _, t := range tasks {
		if t.Source != "" && t.Source != "internal" {
			continue
		}
		out = append(out, internalToUnified(t))
	}
	return out, nil
}

func (p *InternalProvider) Get(ctx context.Context, taskID string) (UnifiedTask, error) {
	t, err := p.Store.Tasks().Get(ctx, taskID)
	if err != nil {
		return UnifiedTask{}, err
	}
	if t.Source != "" && t.Source != "internal" {
		return UnifiedTask{}, data.ErrNotFound
	}
	return internalToUnified(t), nil
}

func (p *InternalProvider) Create(ctx context.Context, in TaskCreateInput) (UnifiedTask, error) {
	var projID, repoID *string
	if in.ProjectID != "" {
		projID = &in.ProjectID
	}
	if in.RepositoryID != "" {
		repoID = &in.RepositoryID
	}
	t, err := p.Store.Tasks().Create(ctx, data.Task{
		Source:       "internal",
		ProjectID:    projID,
		RepositoryID: repoID,
		Title:        in.Title,
		Description:  in.Description,
		Stage:        "inbox",
	})
	if err != nil {
		return UnifiedTask{}, err
	}
	return internalToUnified(t), nil
}

func (p *InternalProvider) Update(ctx context.Context, taskID string, in TaskUpdateInput) (UnifiedTask, error) {
	fields := data.Fields{}
	if in.Title != nil {
		fields["title"] = *in.Title
	}
	if in.Description != nil {
		fields["description"] = *in.Description
	}
	if len(fields) == 0 {
		return p.Get(ctx, taskID)
	}
	t, err := p.Store.Tasks().Patch(ctx, taskID, fields)
	if err != nil {
		return UnifiedTask{}, err
	}
	return internalToUnified(t), nil
}

func (p *InternalProvider) SetState(ctx context.Context, taskID string, in TaskStateInput) (UnifiedTask, error) {
	stage := in.State
	if stage == "open" {
		stage = "inbox"
	}
	t, err := p.Store.MoveStage(ctx, taskID, stage)
	if err != nil {
		return UnifiedTask{}, err
	}
	return internalToUnified(t), nil
}

func (p *InternalProvider) Comment(context.Context, string, string) (github.IssueComment, error) {
	return github.IssueComment{}, data.ErrInvalid.Msgf("internal tasks do not support comments")
}

func (p *InternalProvider) ListComments(context.Context, string) ([]github.IssueComment, error) {
	return nil, nil
}

func internalToUnified(t data.Task) UnifiedTask {
	return UnifiedTask{
		ID:           t.ID,
		Source:       "internal",
		ProjectID:    t.ProjectID,
		RepositoryID: t.RepositoryID,
		Title:        t.Title,
		Description:  t.Description,
		Stage:        t.Stage,
		Position:     t.Position,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}

// GitHubProvider serves GitHub issues backed by external_tasks_cache and tasks(source='github').
type GitHubProvider struct {
	Store     *data.Store
	GitHub    *github.Client
	OnChanged func()
}

func (p *GitHubProvider) Source() string { return "github" }

func (p *GitHubProvider) List(ctx context.Context, filter TaskFilter) ([]UnifiedTask, error) {
	repos, err := p.Store.Repositories().ListAll(ctx, data.Filter{})
	if err != nil {
		return nil, err
	}
	repoProject := map[string]string{}
	for _, r := range repos {
		repoProject[r.ID] = r.ProjectID
	}

	taskRows, err := p.Store.Tasks().ListAll(ctx, data.Filter{Fields: map[string]any{"source": "github"}})
	if err != nil {
		return nil, err
	}
	byID := map[string]data.Task{}
	for _, t := range taskRows {
		byID[t.ID] = t
	}

	rows, err := p.Store.LocalDB().QueryContext(ctx, `
		SELECT task_id, source, repository_id, repo, number, title, body, state, state_reason,
		       author, assignees, labels, label_colors, milestone, url, comments, closing_prs,
		       created_at, updated_at, closed_at
		FROM external_tasks_cache
		WHERE source = 'github'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UnifiedTask{}
	for rows.Next() {
		var u UnifiedTask
		var repoID string
		var num int64
		var assigneesJSON, labelsJSON, colorsJSON, closingJSON string
		if err := rows.Scan(
			&u.ID, &u.Source, &repoID, &u.Repo, &num, &u.Title, &u.Description, &u.State, &u.StateReason,
			&u.Author, &assigneesJSON, &labelsJSON, &colorsJSON, &u.Milestone, &u.URL, &u.Comments, &closingJSON,
			&u.CreatedAt, &u.UpdatedAt, &u.ClosedAt,
		); err != nil {
			return nil, err
		}
		u.RepositoryID = &repoID
		u.Number = &num
		if projID := repoProject[repoID]; projID != "" {
			u.ProjectID = &projID
		}
		if filter.ProjectID != "" && (u.ProjectID == nil || *u.ProjectID != filter.ProjectID) {
			continue
		}
		_ = json.Unmarshal([]byte(assigneesJSON), &u.Assignees)
		_ = json.Unmarshal([]byte(labelsJSON), &u.Labels)
		_ = json.Unmarshal([]byte(colorsJSON), &u.LabelColors)
		_ = json.Unmarshal([]byte(closingJSON), &u.ClosingPRs)

		if filter.Scope != "" && filter.Scope != "all" && filter.Viewer != "" {
			if !matchesScope(u.Assignees, filter.Scope, filter.Viewer) {
				continue
			}
		}
		if row, ok := byID[u.ID]; ok {
			u.Stage = row.Stage
			u.Position = row.Position
			if row.ProjectID != nil {
				u.ProjectID = row.ProjectID
			}
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func matchesScope(assignees []string, scope, viewer string) bool {
	mine := slices.ContainsFunc(assignees, func(a string) bool { return strings.EqualFold(a, viewer) })
	switch scope {
	case "mine":
		return mine
	case "mine_or_unassigned":
		return mine || len(assignees) == 0
	default:
		return true
	}
}

func (p *GitHubProvider) Get(ctx context.Context, taskID string) (UnifiedTask, error) {
	list, err := p.List(ctx, TaskFilter{})
	if err != nil {
		return UnifiedTask{}, err
	}
	for _, u := range list {
		if u.ID == taskID {
			return u, nil
		}
	}
	return UnifiedTask{}, data.ErrNotFound
}

func (p *GitHubProvider) Create(ctx context.Context, in TaskCreateInput) (UnifiedTask, error) {
	if p.GitHub == nil {
		return UnifiedTask{}, github.ErrUnconfigured
	}
	repo, err := p.Store.Repositories().Get(ctx, in.RepositoryID)
	if err != nil {
		return UnifiedTask{}, err
	}
	repoName, ok := github.ParseRemote(repo.RemoteURL)
	if !ok {
		return UnifiedTask{}, data.ErrInvalid.Msgf("repository is not a GitHub repository")
	}
	iss, body, err := p.GitHub.CreateIssue(ctx, repoName, github.IssueCreateInput{
		Title:     in.Title,
		Body:      in.Description,
		Labels:    in.Labels,
		Assignees: in.Assignees,
	})
	if err != nil {
		return UnifiedTask{}, err
	}
	if err := UpsertExternalTaskCache(ctx, p.Store.LocalDB(), repo.ID, repoName, iss, body); err != nil {
		return UnifiedTask{}, err
	}
	if _, err := p.Store.EnsureExternalTask(ctx, &repo.ProjectID, repo.ID, int64(iss.Number)); err != nil {
		return UnifiedTask{}, err
	}
	if p.OnChanged != nil {
		p.OnChanged()
	}
	return p.Get(ctx, data.ExternalTaskID(repo.ID, int64(iss.Number)))
}

func (p *GitHubProvider) Update(ctx context.Context, taskID string, in TaskUpdateInput) (UnifiedTask, error) {
	if p.GitHub == nil {
		return UnifiedTask{}, github.ErrUnconfigured
	}
	u, err := p.Get(ctx, taskID)
	if err != nil || u.Number == nil || u.RepositoryID == nil {
		return UnifiedTask{}, data.ErrNotFound
	}
	iss, body, err := p.GitHub.UpdateIssue(ctx, u.Repo, int(*u.Number), github.IssueUpdateInput{
		Title:     in.Title,
		Body:      in.Description,
		Labels:    in.Labels,
		Assignees: in.Assignees,
	})
	if err != nil {
		return UnifiedTask{}, err
	}
	if err := UpsertExternalTaskCache(ctx, p.Store.LocalDB(), *u.RepositoryID, u.Repo, iss, body); err != nil {
		return UnifiedTask{}, err
	}
	if p.OnChanged != nil {
		p.OnChanged()
	}
	return p.Get(ctx, taskID)
}

func (p *GitHubProvider) SetState(ctx context.Context, taskID string, in TaskStateInput) (UnifiedTask, error) {
	if p.GitHub == nil {
		return UnifiedTask{}, github.ErrUnconfigured
	}
	u, err := p.Get(ctx, taskID)
	if err != nil || u.Number == nil || u.RepositoryID == nil {
		return UnifiedTask{}, data.ErrNotFound
	}
	ghState := "open"
	var reason *string
	switch in.State {
	case "done":
		ghState = "closed"
		r := "completed"
		if in.Reason != "" {
			r = strings.ToLower(in.Reason)
		}
		reason = &r
	case "cancelled":
		ghState = "closed"
		r := "not_planned"
		reason = &r
	case "open":
		ghState = "open"
		r := "reopened"
		reason = &r
	default:
		return UnifiedTask{}, data.ErrInvalid.Msgf("invalid state %q", in.State)
	}
	iss, body, err := p.GitHub.UpdateIssue(ctx, u.Repo, int(*u.Number), github.IssueUpdateInput{
		State:       &ghState,
		StateReason: reason,
	})
	if err != nil {
		return UnifiedTask{}, err
	}
	if err := UpsertExternalTaskCache(ctx, p.Store.LocalDB(), *u.RepositoryID, u.Repo, iss, body); err != nil {
		return UnifiedTask{}, err
	}
	if p.OnChanged != nil {
		p.OnChanged()
	}
	return p.Get(ctx, taskID)
}

func (p *GitHubProvider) Comment(ctx context.Context, taskID, body string) (github.IssueComment, error) {
	if p.GitHub == nil {
		return github.IssueComment{}, github.ErrUnconfigured
	}
	u, err := p.Get(ctx, taskID)
	if err != nil || u.Number == nil {
		return github.IssueComment{}, data.ErrNotFound
	}
	c, err := p.GitHub.CommentIssue(ctx, u.Repo, int(*u.Number), body)
	if err == nil && p.OnChanged != nil {
		p.OnChanged()
	}
	return c, err
}

func (p *GitHubProvider) ListComments(ctx context.Context, taskID string) ([]github.IssueComment, error) {
	if p.GitHub == nil {
		return nil, github.ErrUnconfigured
	}
	u, err := p.Get(ctx, taskID)
	if err != nil || u.Number == nil {
		return nil, data.ErrNotFound
	}
	return p.GitHub.ListIssueComments(ctx, u.Repo, int(*u.Number))
}

// UpsertExternalTaskCache writes one GitHub issue into external_tasks_cache.
func UpsertExternalTaskCache(ctx context.Context, db *sql.DB, repositoryID, repo string, iss github.Issue, body string) error {
	taskID := data.ExternalTaskID(repositoryID, int64(iss.Number))
	assignees, _ := json.Marshal(iss.Assignees)
	labels, _ := json.Marshal(iss.Labels)
	colors, _ := json.Marshal(iss.LabelColors)
	closing := make([]int, 0, len(iss.Pulls))
	for _, pr := range iss.Pulls {
		closing = append(closing, pr.Number)
	}
	closingJSON, _ := json.Marshal(closing)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO external_tasks_cache(
			task_id, source, repository_id, repo, number, node_id, title, body, state, state_reason,
			author, assignees, labels, label_colors, milestone, url, comments, closing_prs,
			created_at, updated_at, closed_at, cached_at
		) VALUES (?, 'github', ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			title = excluded.title,
			body = CASE WHEN excluded.body != '' THEN excluded.body ELSE external_tasks_cache.body END,
			state = excluded.state,
			state_reason = excluded.state_reason,
			author = excluded.author,
			assignees = excluded.assignees,
			labels = excluded.labels,
			label_colors = excluded.label_colors,
			milestone = excluded.milestone,
			url = excluded.url,
			comments = excluded.comments,
			closing_prs = excluded.closing_prs,
			updated_at = excluded.updated_at,
			closed_at = excluded.closed_at,
			cached_at = excluded.cached_at
	`, taskID, repositoryID, repo, iss.Number, iss.Title, body, strings.ToLower(iss.State), strings.ToUpper(iss.StateReason),
		iss.Author, string(assignees), string(labels), string(colors), iss.Milestone, iss.URL, iss.Comments, string(closingJSON),
		iss.CreatedAt, iss.UpdatedAt, iss.ClosedAt, now)
	return err
}

// RefreshExternalTasks syncs open GitHub issues from the GitHub overview into
// external_tasks_cache and marks previously open issues that disappeared as closed.
func (a *Tasks) RefreshExternalTasks(ctx context.Context, source TaskPullSource) (int, error) {
	if source == nil {
		return 0, nil
	}
	overview, err := source.Open(ctx, false)
	if err != nil {
		return 0, err
	}
	if overview == nil || (overview.Refreshing && len(overview.Issues) == 0) {
		return 0, nil
	}
	repos, err := a.Store.Repositories().ListAll(ctx, data.Filter{})
	if err != nil {
		return 0, err
	}
	repoIDByName := map[string]string{}
	for _, r := range repos {
		if name, ok := github.ParseRemote(r.RemoteURL); ok {
			repoIDByName[strings.ToLower(name)] = r.ID
		}
	}
	if len(repoIDByName) == 0 {
		return 0, nil
	}

	seen := map[string]bool{}
	count := 0
	for _, iss := range overview.Issues {
		repoID := repoIDByName[strings.ToLower(iss.Repo)]
		if repoID == "" {
			continue
		}
		if err := UpsertExternalTaskCache(ctx, a.Store.LocalDB(), repoID, iss.Repo, iss, ""); err != nil {
			return count, err
		}
		seen[data.ExternalTaskID(repoID, int64(iss.Number))] = true
		count++
	}

	// Mark open cached issues that are no longer in the open overview as closed.
	rows, err := a.Store.LocalDB().QueryContext(ctx, `SELECT task_id FROM external_tasks_cache WHERE source = 'github' AND state = 'open'`)
	if err != nil {
		return count, err
	}
	var toClose []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err == nil && !seen[tid] {
			toClose = append(toClose, tid)
		}
	}
	rows.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, tid := range toClose {
		if _, err := a.Store.LocalDB().ExecContext(ctx,
			`UPDATE external_tasks_cache SET state = 'closed', cached_at = ? WHERE task_id = ?`, now, tid); err != nil {
			return count, err
		}
	}
	return count, nil
}

// DetectThreadPullsByBranch links PRs found in task_pulls whose head_ref matches
// a local thread's branch, respecting manual unlinks (archived thread_pulls rows).
func (a *Tasks) DetectThreadPullsByBranch(ctx context.Context, nodeID string) error {
	if nodeID == "" {
		return nil
	}
	threads, err := a.Store.Threads().ListAll(ctx, data.Filter{Fields: map[string]any{"node_id": nodeID}})
	if err != nil {
		return err
	}
	repos, err := a.Store.Repositories().ListAll(ctx, data.Filter{})
	if err != nil {
		return err
	}
	repoByID := map[string]data.Repository{}
	repoIDByName := map[string]string{}
	for _, r := range repos {
		repoByID[r.ID] = r
		if name, ok := github.ParseRemote(r.RemoteURL); ok {
			repoIDByName[strings.ToLower(name)] = r.ID
		}
	}
	existing, err := a.Store.ThreadPulls().ListAll(ctx, data.Filter{IncludeArchived: true})
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, tp := range existing {
		seen[fmt.Sprintf("%s/%s/%d", tp.ThreadID, tp.RepositoryID, tp.Number)] = true
	}

	// Build head_ref -> []{repoID, number} from task_pulls
	type prMatch struct {
		repoID string
		number int64
	}
	byHead := map[string][]prMatch{}
	rows, err := a.Store.LocalDB().QueryContext(ctx, `SELECT repo, number, head_ref FROM task_pulls WHERE head_ref != '' AND number IS NOT NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var repoName, headRef string
		var num int64
		if err := rows.Scan(&repoName, &num, &headRef); err == nil {
			if rid := repoIDByName[strings.ToLower(repoName)]; rid != "" {
				byHead[headRef] = append(byHead[headRef], prMatch{repoID: rid, number: num})
			}
		}
	}
	rows.Close()

	for _, th := range threads {
		if th.TaskID == nil || th.Branch == "" {
			continue
		}
		matches := byHead[th.Branch]
		for _, m := range matches {
			if r, ok := repoByID[m.repoID]; ok && r.DefaultBranch == th.Branch {
				continue
			}
			key := fmt.Sprintf("%s/%s/%d", th.ID, m.repoID, m.number)
			if seen[key] {
				continue // already linked or manually unlinked (archived)
			}
			if _, err := a.Store.ThreadPulls().Link(ctx, th.ID, m.repoID, m.number, th.Branch, "branch"); err != nil {
				return err
			}
			seen[key] = true
		}
	}
	return nil
}

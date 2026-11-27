package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
)

// UnifiedTask is the source-agnostic representation of a task (internal or external).
type UnifiedTask struct {
	ID     string `json:"id"`
	Source string `json:"source"` // internal | github
	// ExternalID is the item's stable id in its provider; empty for internal tasks.
	ExternalID string `json:"external_id,omitempty"`
	// Adopted says a Dory task holds the item; an external item that is not
	// adopted is only shown from the local cache of the node.
	Adopted        bool              `json:"adopted"`
	ProjectID      *string           `json:"project_id,omitempty"`
	RepositoryID   *string           `json:"repository_id,omitempty"`
	Repo           string            `json:"repo,omitempty"`
	Number         *int64            `json:"number,omitempty"`
	Title          string            `json:"title"`
	Description    string            `json:"description"`
	URL            string            `json:"url,omitempty"`
	State          string            `json:"state,omitempty"`        // open | closed
	StateReason    string            `json:"state_reason,omitempty"` // COMPLETED | NOT_PLANNED | ""
	Stage          string            `json:"stage"`
	Position       string            `json:"position"`
	Author         string            `json:"author,omitempty"`
	Assignees      []string          `json:"assignees,omitempty"`
	Labels         []string          `json:"labels,omitempty"`
	LabelColors    map[string]string `json:"label_colors,omitempty"`
	Milestone      string            `json:"milestone,omitempty"`
	Comments       int               `json:"comments,omitempty"`
	ClosingPRs     []int             `json:"closing_prs,omitempty"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	ClosedAt       string            `json:"closed_at,omitempty"`
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
	// TaskID, when set, is the Dory task the new external item belongs to.
	TaskID       string
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
		ID:             t.ID,
		Source:         "internal",
		Adopted:        true,
		ProjectID:      t.ProjectID,
		RepositoryID:   t.RepositoryID,
		Number:         t.Number,
		Title:          t.Title,
		Description:    t.Description,
		Stage:          t.Stage,
		Position:       t.Position,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		legacyBranch:   t.Branch,
		legacyPRNumber: t.PRNumber,
	}
}

// GitHubProvider serves GitHub issues: the adopted ones from their task rows
// and the rest from this node's external_tasks_cache.
type GitHubProvider struct {
	Store     *data.Store
	GitHub    *github.Client
	OnChanged func()
}

func (p *GitHubProvider) Source() string { return "github" }

const cacheColumns = `task_id, source, external_id, repository_id, repo, number, title, body, state, state_reason,
	author, assignees, labels, label_colors, milestone, url, comments, closing_prs, created_at, updated_at, closed_at`

func scanCached(rows interface{ Scan(...any) error }) (UnifiedTask, error) {
	var u UnifiedTask
	var repoID string
	var num int64
	var assigneesJSON, labelsJSON, colorsJSON, closingJSON string
	if err := rows.Scan(
		&u.ID, &u.Source, &u.ExternalID, &repoID, &u.Repo, &num, &u.Title, &u.Description, &u.State, &u.StateReason,
		&u.Author, &assigneesJSON, &labelsJSON, &colorsJSON, &u.Milestone, &u.URL, &u.Comments, &closingJSON,
		&u.CreatedAt, &u.UpdatedAt, &u.ClosedAt,
	); err != nil {
		return u, err
	}
	u.RepositoryID, u.Number = &repoID, &num
	_ = json.Unmarshal([]byte(assigneesJSON), &u.Assignees)
	_ = json.Unmarshal([]byte(labelsJSON), &u.Labels)
	_ = json.Unmarshal([]byte(colorsJSON), &u.LabelColors)
	_ = json.Unmarshal([]byte(closingJSON), &u.ClosingPRs)
	return u, nil
}

// adopt applies the task that holds a cached item: its id and manual state win.
func (u *UnifiedTask) adopt(t data.Task) {
	u.ID, u.Adopted, u.Stage, u.Position = t.ID, true, t.Stage, t.Position
	if t.ProjectID != nil {
		u.ProjectID = t.ProjectID
	}
}

func (p *GitHubProvider) List(ctx context.Context, filter TaskFilter) ([]UnifiedTask, error) {
	repos, err := p.Store.Repositories().ListAll(ctx, data.Filter{})
	if err != nil {
		return nil, err
	}
	repoProject, repoName := map[string]string{}, map[string]string{}
	for _, r := range repos {
		repoProject[r.ID] = r.ProjectID
		repoName[r.ID], _ = github.ParseRemote(r.RemoteURL)
	}
	taskRows, err := p.Store.Tasks().ListAll(ctx, data.Filter{Fields: map[string]any{"source": "github"}})
	if err != nil {
		return nil, err
	}
	byKey := map[string]data.Task{}
	for _, t := range taskRows {
		if t.ExternalID != "" {
			byKey[t.ExternalID] = t
		}
	}

	rows, err := p.Store.LocalDB().QueryContext(ctx, `SELECT `+cacheColumns+` FROM external_tasks_cache WHERE source = 'github'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UnifiedTask{}
	keep := func(u UnifiedTask) bool {
		return filter.ProjectID == "" || (u.ProjectID != nil && *u.ProjectID == filter.ProjectID)
	}
	for rows.Next() {
		u, err := scanCached(rows)
		if err != nil {
			return nil, err
		}
		if projID := repoProject[*u.RepositoryID]; projID != "" {
			u.ProjectID = &projID
		}
		if t, ok := byKey[u.ExternalID]; ok {
			u.adopt(t)
			delete(byKey, u.ExternalID)
		} else if u.State != "open" {
			continue // a closed item nobody adopted is not work for Dory
		} else if filter.Scope != "" && filter.Scope != "all" && filter.Viewer != "" && !matchesScope(u.Assignees, filter.Scope, filter.Viewer) {
			continue
		}
		if keep(u) {
			out = append(out, u)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Adopted tasks this node has no cache for still show, from their row.
	for _, t := range byKey {
		u := UnifiedTask{ID: t.ID, Source: t.Source, ExternalID: t.ExternalID, Adopted: true, ProjectID: t.ProjectID,
			RepositoryID: t.RepositoryID, Number: t.Number, Title: t.Title, Description: t.Description,
			Stage: t.Stage, Position: t.Position, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
		if t.RepositoryID != nil {
			u.Repo = repoName[*t.RepositoryID]
		}
		if keep(u) {
			out = append(out, u)
		}
	}
	return out, nil
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
	// An issue Dory creates is Dory's work: it belongs to its task at once.
	ref := issueRef(&repo.ProjectID, repo.ID, iss, body)
	var t data.Task
	if in.TaskID != "" {
		t, err = p.Store.AttachExternal(ctx, in.TaskID, ref)
	} else {
		t, err = p.Store.AdoptTask(ctx, ref)
	}
	if err != nil {
		return UnifiedTask{}, err
	}
	if p.OnChanged != nil {
		p.OnChanged()
	}
	return p.Get(ctx, t.ID)
}

// issueRef is the key and data of a GitHub issue for the task that holds it.
func issueRef(projectID *string, repositoryID string, iss github.Issue, body string) data.ExternalRef {
	return data.ExternalRef{Source: "github", ExternalID: iss.NodeID, ProjectID: projectID, RepositoryID: repositoryID,
		Number: int64(iss.Number), Title: iss.Title, Description: body}
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
	if iss.NodeID == "" {
		return data.ErrInvalid.Msgf("issue %s#%d has no node id", repo, iss.Number)
	}
	taskID := data.AdoptedTaskID("github", iss.NodeID)
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
			task_id, source, external_id, repository_id, repo, number, title, body, state, state_reason,
			author, assignees, labels, label_colors, milestone, url, comments, closing_prs,
			created_at, updated_at, closed_at, cached_at
		) VALUES (?, 'github', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			repository_id = excluded.repository_id,
			repo = excluded.repo,
			number = excluded.number,
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
	`, taskID, iss.NodeID, repositoryID, repo, iss.Number, iss.Title, body, strings.ToLower(iss.State), strings.ToUpper(iss.StateReason),
		iss.Author, string(assignees), string(labels), string(colors), iss.Milestone, iss.URL, iss.Comments, string(closingJSON),
		iss.CreatedAt, iss.UpdatedAt, iss.ClosedAt, now)
	return err
}

// IssueSource gives the issues the GitHub watcher last read.
type IssueSource interface {
	IssueSnapshots() []github.IssueSnapshot
}

// RefreshExternalTasks copies the issues the watcher read into this node's
// external_tasks_cache. It never creates tasks. An open issue that is no
// longer listed leaves the cache only when its repository listed every open
// issue; a closed issue stays only while a task holds it.
func (a *Tasks) RefreshExternalTasks(ctx context.Context, source IssueSource) (int, error) {
	if source == nil {
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
	db := a.Store.LocalDB()
	count := 0
	for _, snap := range source.IssueSnapshots() {
		repoID := repoIDByName[strings.ToLower(snap.Repo)]
		if repoID == "" {
			continue
		}
		seen := map[string]bool{}
		for _, iss := range append(slices.Clone(snap.Open), snap.Closed...) {
			if iss.NodeID == "" {
				continue
			}
			if err := UpsertExternalTaskCache(ctx, db, repoID, snap.Repo, iss, ""); err != nil {
				return count, err
			}
			seen[iss.NodeID] = true
			count++
		}
		if !snap.Complete {
			continue
		}
		rows, err := db.QueryContext(ctx, `SELECT external_id FROM external_tasks_cache WHERE source = 'github' AND repository_id = ? AND state = 'open'`, repoID)
		if err != nil {
			return count, err
		}
		var gone []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil && !seen[id] {
				gone = append(gone, id)
			}
		}
		rows.Close()
		for _, id := range gone {
			if _, err := db.ExecContext(ctx, `DELETE FROM external_tasks_cache WHERE source = 'github' AND external_id = ?`, id); err != nil {
				return count, err
			}
		}
	}
	_, err = db.ExecContext(ctx, `DELETE FROM external_tasks_cache WHERE state != 'open' AND NOT EXISTS (
		SELECT 1 FROM tasks t WHERE t.source = external_tasks_cache.source AND t.external_id = external_tasks_cache.external_id AND t.archived_at IS NULL)`)
	return count, err
}

// cachedRef is the cached external item whose adopted id is id.
func (a *Tasks) cachedRef(ctx context.Context, id string) (data.ExternalRef, bool, error) {
	var ref data.ExternalRef
	var number int64
	err := a.Store.LocalDB().QueryRowContext(ctx,
		`SELECT source, external_id, repository_id, number, title, body FROM external_tasks_cache WHERE task_id = ?`, id,
	).Scan(&ref.Source, &ref.ExternalID, &ref.RepositoryID, &number, &ref.Title, &ref.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return ref, false, nil
	}
	if err != nil {
		return ref, false, err
	}
	ref.Number = number
	if r, err := a.Store.Repositories().Get(ctx, ref.RepositoryID); err == nil {
		ref.ProjectID = &r.ProjectID
	}
	return ref, true, nil
}

// Adopt returns the task row of id. When id names a cached external item
// that no task holds yet, it adopts the item first. Every write on a task
// goes through here, so an item becomes a Dory task in one place only.
func (a *Tasks) Adopt(ctx context.Context, id string) (data.Task, error) {
	t, err := a.Store.Tasks().Get(ctx, id)
	if !errors.Is(err, data.ErrNotFound) {
		return t, err
	}
	ref, ok, err := a.cachedRef(ctx, id)
	if err != nil {
		return data.Task{}, err
	}
	if !ok {
		return data.Task{}, data.ErrNotFound
	}
	return a.Store.AdoptTask(ctx, ref)
}

// Known says whether id names a task or a cached external item.
func (a *Tasks) Known(ctx context.Context, id string) bool {
	_, _, err := a.ProviderFor(ctx, id)
	return err == nil
}

// AdoptIssue adopts an issue of a registered repository, reading it from
// GitHub when this node has not cached it.
func (a *Tasks) AdoptIssue(ctx context.Context, repositoryID string, number int64) (data.Task, error) {
	ref, err := a.issueRef(ctx, repositoryID, number)
	if err != nil {
		return data.Task{}, err
	}
	return a.Store.AdoptTask(ctx, ref)
}

// AttachIssue gives task id the key of an issue of its repository.
func (a *Tasks) AttachIssue(ctx context.Context, id, repositoryID string, number int64) (data.Task, error) {
	ref, err := a.issueRef(ctx, repositoryID, number)
	if err != nil {
		return data.Task{}, err
	}
	return a.Store.AttachExternal(ctx, id, ref)
}

func (a *Tasks) issueRef(ctx context.Context, repositoryID string, number int64) (data.ExternalRef, error) {
	var taskID string
	err := a.Store.LocalDB().QueryRowContext(ctx,
		`SELECT task_id FROM external_tasks_cache WHERE source = 'github' AND repository_id = ? AND number = ?`, repositoryID, number).Scan(&taskID)
	if err == nil {
		ref, ok, err := a.cachedRef(ctx, taskID)
		if err != nil || ok {
			return ref, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return data.ExternalRef{}, err
	}
	gp := a.githubProvider()
	if gp == nil || gp.GitHub == nil {
		return data.ExternalRef{}, github.ErrUnconfigured
	}
	repo, err := a.Store.Repositories().Get(ctx, repositoryID)
	if err != nil {
		return data.ExternalRef{}, err
	}
	name, ok := github.ParseRemote(repo.RemoteURL)
	if !ok {
		return data.ExternalRef{}, data.ErrInvalid.Msgf("repository is not a GitHub repository")
	}
	iss, body, err := gp.GitHub.GetIssue(ctx, name, int(number))
	if err != nil {
		return data.ExternalRef{}, err
	}
	if err := UpsertExternalTaskCache(ctx, a.Store.LocalDB(), repo.ID, name, iss, body); err != nil {
		return data.ExternalRef{}, err
	}
	return issueRef(&repo.ProjectID, repo.ID, iss, body), nil
}

func (a *Tasks) githubProvider() *GitHubProvider {
	for _, p := range a.providers() {
		if gp, ok := p.(*GitHubProvider); ok {
			return gp
		}
	}
	return nil
}

// repositoryByName is the registered repository of a GitHub owner/name.
func (a *Tasks) repositoryByName(ctx context.Context, name string) (data.Repository, error) {
	repos, err := a.Store.Repositories().ListAll(ctx, data.Filter{})
	if err != nil {
		return data.Repository{}, err
	}
	for _, r := range repos {
		if n, ok := github.ParseRemote(r.RemoteURL); ok && strings.EqualFold(n, name) {
			return r, nil
		}
	}
	return data.Repository{}, data.ErrNotFound.Msgf("repository not registered: %s", name)
}

// AdoptRepoIssue adopts issue number of the GitHub repository name.
func (a *Tasks) AdoptRepoIssue(ctx context.Context, name string, number int64) (data.Task, error) {
	r, err := a.repositoryByName(ctx, name)
	if err != nil {
		return data.Task{}, err
	}
	return a.AdoptIssue(ctx, r.ID, number)
}

// IssueTaskID is the id an issue shows under: the task that holds it, or its
// cached item. It never creates a task.
func (a *Tasks) IssueTaskID(ctx context.Context, name string, number int64) (string, error) {
	r, err := a.repositoryByName(ctx, name)
	if err != nil {
		return "", err
	}
	held, err := a.Store.Tasks().ListAll(ctx, data.Filter{Fields: map[string]any{"source": "github", "repository_id": r.ID, "number": number}})
	if err != nil {
		return "", err
	}
	if len(held) > 0 {
		return held[0].ID, nil
	}
	var id string
	err = a.Store.LocalDB().QueryRowContext(ctx,
		`SELECT task_id FROM external_tasks_cache WHERE source = 'github' AND repository_id = ? AND number = ?`, r.ID, number).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", data.ErrNotFound.Msgf("issue %s#%d is not known on this node", name, number)
	}
	return id, err
}

// DetectIssueLinks gives an internal task the key of the issue that a pull
// request of its threads closes, so the issue does not show as a second card.
// Only the node that runs the thread writes the link.
func (a *Tasks) DetectIssueLinks(ctx context.Context, nodeID string) error {
	if nodeID == "" {
		return nil
	}
	rows, err := a.Store.LocalDB().QueryContext(ctx, `
		SELECT DISTINCT t.id, c.repository_id, c.number
		FROM external_tasks_cache c, json_each(c.closing_prs) j
		JOIN thread_pulls tp ON tp.repository_id = c.repository_id AND tp.number = j.value AND tp.archived_at IS NULL
		JOIN threads th ON th.id = tp.thread_id AND th.archived_at IS NULL AND th.node_id = ?
		JOIN tasks t ON t.id = th.task_id AND t.archived_at IS NULL AND t.source = 'internal'
		WHERE c.source = 'github' AND NOT EXISTS (
			SELECT 1 FROM tasks o WHERE o.source = c.source AND o.external_id = c.external_id AND o.archived_at IS NULL)`, nodeID)
	if err != nil {
		return err
	}
	type link struct {
		task, repo string
		number     int64
	}
	var links []link
	for rows.Next() {
		var l link
		if rows.Scan(&l.task, &l.repo, &l.number) == nil {
			links = append(links, l)
		}
	}
	rows.Close()
	attached := map[string]bool{}
	for _, l := range links {
		if attached[l.task] {
			continue // a task holds one issue
		}
		if _, err := a.AttachIssue(ctx, l.task, l.repo, l.number); err != nil && !errors.Is(err, data.ErrConflict) {
			return err
		}
		attached[l.task] = true
	}
	return nil
}

// ArchiveImportedTasks archives the external tasks created before adoption,
// which have no external key. It is idempotent.
func (a *Tasks) ArchiveImportedTasks(ctx context.Context) error {
	rows, err := a.Store.Tasks().ListAll(ctx, data.Filter{Fields: map[string]any{"external_id": ""}})
	if err != nil {
		return err
	}
	for _, t := range rows {
		if t.Source == "" || t.Source == "internal" {
			continue
		}
		if err := a.Store.Tasks().Archive(ctx, t.ID); err != nil {
			return err
		}
	}
	return nil
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

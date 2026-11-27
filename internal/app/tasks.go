package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/domain/board"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/notify"
)

// Tasks coordinates task writes, launches and the local board.
type Tasks struct {
	Store           *data.Store
	Threads         *Threads
	Plans           *Plans
	Node            string
	ArchiveOnFinish func() bool
	GitHub          interface {
		Configured(context.Context) bool
		PullState(context.Context, string, *int64, string) (github.PullStatus, error)
	}

	pullCursor int // owned by the GitHub watcher
	finishing  sync.Mutex
	finishSoon debounce
}

// CardPull is a pull request associated with a task card, either via thread_pulls
// or externally via "Closes #N".
type CardPull struct {
	Repo           string   `json:"repo"`
	RepositoryID   string   `json:"repository_id,omitempty"`
	Number         int      `json:"number"`
	Title          string   `json:"title,omitempty"`
	URL            string   `json:"url,omitempty"`
	State          string   `json:"state"` // draft | open | merged | closed
	CI             string   `json:"ci,omitempty"`
	ReviewDecision string   `json:"review_decision,omitempty"`
	ReviewRequests []string `json:"review_requests,omitempty"`
	ThreadIDs      []string `json:"thread_ids,omitempty"`
	External       bool     `json:"external,omitempty"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
}

// Card is the wire representation of a task with its derived board state.
type Card struct {
	data.Task
	Column        string            `json:"column"`
	Reason        string            `json:"reason,omitempty"`
	Repo          string            `json:"repo,omitempty"`
	URL           string            `json:"url,omitempty"`
	ExternalState string            `json:"external_state,omitempty"`
	StateReason   string            `json:"state_reason,omitempty"`
	Author        string            `json:"author,omitempty"`
	Assignees     []string          `json:"assignees,omitempty"`
	Labels        []string          `json:"labels,omitempty"`
	LabelColors   map[string]string `json:"label_colors,omitempty"`
	Milestone     string            `json:"milestone,omitempty"`
	Comments      int               `json:"comments,omitempty"`
	Threads       []data.Thread     `json:"threads"`
	Pull          *github.Pull      `json:"pull,omitempty"`
	Pulls         []CardPull        `json:"pulls"`
	PRState       string            `json:"pr_state,omitempty"`
	Blockers      []string          `json:"blockers"`
	HasPlan       bool              `json:"has_plan"`
	PlanAccepted  bool              `json:"plan_accepted"`
}

func (a *Tasks) Get(ctx context.Context, id string) (data.Task, error) {
	return a.Store.Tasks().Get(ctx, id)
}

func (a *Tasks) List(ctx context.Context, f data.Filter) ([]data.Task, error) {
	return a.Store.Tasks().ListAll(ctx, f)
}

func (a *Tasks) Card(ctx context.Context, id string) (Card, error) {
	t, err := a.Get(ctx, id)
	if err != nil {
		return Card{}, err
	}
	cards, err := a.cards(ctx, []data.Task{t})
	if err != nil {
		return Card{}, err
	}
	return cards[0], nil
}

func (a *Tasks) Board(ctx context.Context, f data.Filter) ([]Card, error) {
	tasks, err := a.List(ctx, f)
	if err != nil {
		return nil, err
	}
	return a.cards(ctx, tasks)
}

func (a *Tasks) repositoryNames(ctx context.Context) (map[string]string, error) {
	repos, err := a.Store.Repositories().ListAll(ctx, data.Filter{})
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(repos))
	for _, r := range repos {
		names[r.ID], _ = github.ParseRemote(r.RemoteURL)
	}
	return names, nil
}

func taskRepo(t data.Task, names map[string]string) string {
	if t.RepositoryID != nil {
		return names[*t.RepositoryID]
	}
	return ""
}

// BoardMulti returns cards from all enabled task providers (internal + github)
// using only local SQLite tables (no GitHub calls, no N+1 queries).
func (a *Tasks) BoardMulti(ctx context.Context, filter TaskFilter) ([]Card, error) {
	var unified []UnifiedTask
	if filter.Source == "" || filter.Source == "all" || filter.Source == "internal" {
		ip := &InternalProvider{Store: a.Store}
		list, err := ip.List(ctx, filter)
		if err != nil {
			return nil, err
		}
		unified = append(unified, list...)
	}
	if filter.Source == "" || filter.Source == "all" || filter.Source == "github" {
		gp := &GitHubProvider{Store: a.Store}
		list, err := gp.List(ctx, filter)
		if err != nil {
			return nil, err
		}
		unified = append(unified, list...)
	}
	cards, err := a.cardsFromUnified(ctx, unified)
	if err != nil {
		return nil, err
	}
	if filter.Open {
		filtered := make([]Card, 0, len(cards))
		for _, c := range cards {
			if c.Column != string(board.Done) && c.Column != string(board.Cancelled) {
				filtered = append(filtered, c)
			}
		}
		cards = filtered
	}
	return cards, nil
}

func (a *Tasks) cards(ctx context.Context, tasks []data.Task) ([]Card, error) {
	if len(tasks) == 0 {
		return []Card{}, nil
	}
	names, err := a.repositoryNames(ctx)
	if err != nil {
		return nil, err
	}
	// Load external cache for any github tasks in the list.
	extByID := map[string]UnifiedTask{}
	gp := &GitHubProvider{Store: a.Store}
	if extList, err := gp.List(ctx, TaskFilter{}); err == nil {
		for _, u := range extList {
			extByID[u.ID] = u
		}
	}
	unified := make([]UnifiedTask, 0, len(tasks))
	for _, t := range tasks {
		if u, ok := extByID[t.ID]; ok {
			u.Stage = t.Stage
			u.Position = t.Position
			unified = append(unified, u)
			continue
		}
		u := internalToUnified(t)
		u.Source = t.Source
		if u.Source == "" {
			u.Source = "internal"
		}
		u.Number = t.Number
		u.Repo = taskRepo(t, names)
		u.legacyBranch = t.Branch
		u.legacyPRNumber = t.PRNumber
		unified = append(unified, u)
	}
	return a.cardsFromUnified(ctx, unified)
}

func (a *Tasks) cardsFromUnified(ctx context.Context, unified []UnifiedTask) ([]Card, error) {
	cards := make([]Card, 0, len(unified))
	if len(unified) == 0 {
		return cards, nil
	}
	threads, err := a.Store.Threads().ListAll(ctx, data.Filter{})
	if err != nil {
		return nil, err
	}
	byTask := map[string][]data.Thread{}
	for _, t := range threads {
		if t.TaskID != nil {
			byTask[*t.TaskID] = append(byTask[*t.TaskID], t)
		}
	}
	threadPulls, err := a.Store.ThreadPulls().ListAll(ctx, data.Filter{})
	if err != nil {
		return nil, err
	}
	tpByThread := map[string][]data.ThreadPull{}
	for _, tp := range threadPulls {
		tpByThread[tp.ThreadID] = append(tpByThread[tp.ThreadID], tp)
	}
	names, err := a.repositoryNames(ctx)
	if err != nil {
		return nil, err
	}
	plans, err := a.Store.Plans().ListAll(ctx, data.Filter{})
	if err != nil {
		return nil, err
	}
	withPlan := map[string]bool{}
	planAccepted := map[string]bool{}
	for _, p := range plans {
		withPlan[p.TaskID] = true
		if p.AcceptedAt != nil {
			planAccepted[p.TaskID] = true
		}
	}
	deps, err := a.Store.BoardDependencies(ctx)
	if err != nil {
		return nil, err
	}
	pulls, err := a.cachedPulls(ctx)
	if err != nil {
		return nil, err
	}

	for _, u := range unified {
		t := data.Task{
			ID:           u.ID,
			Source:       u.Source,
			ProjectID:    u.ProjectID,
			RepositoryID: u.RepositoryID,
			Number:       u.Number,
			Title:        u.Title,
			Description:  u.Description,
			Stage:        u.Stage,
			Branch:       u.legacyBranch,
			PRNumber:     u.legacyPRNumber,
			Position:     u.Position,
			CreatedAt:    u.CreatedAt,
			UpdatedAt:    u.UpdatedAt,
		}
		repo := u.Repo
		if repo == "" {
			repo = taskRepo(t, names)
		}
		c := Card{
			Task:          t,
			Repo:          repo,
			URL:           u.URL,
			ExternalState: u.State,
			StateReason:   u.StateReason,
			Author:        u.Author,
			Assignees:     u.Assignees,
			Labels:        u.Labels,
			LabelColors:   u.LabelColors,
			Milestone:     u.Milestone,
			Comments:      u.Comments,
			Threads:       byTask[u.ID],
			Pulls:         []CardPull{},
			HasPlan:       withPlan[u.ID],
			PlanAccepted:  planAccepted[u.ID],
			Blockers:      []string{},
		}
		if c.Threads == nil {
			c.Threads = []data.Thread{}
		}

		// Deduplicate PRs by (repo, number): thread_pulls first, then closing_prs.
		type prKey struct {
			repo   string
			number int
		}
		prIndex := map[prKey]int{}
		addPR := func(rName, repoID string, num int, threadID string, external bool) {
			k := prKey{repo: rName, number: num}
			if idx, ok := prIndex[k]; ok {
				if threadID != "" {
					c.Pulls[idx].ThreadIDs = append(c.Pulls[idx].ThreadIDs, threadID)
					c.Pulls[idx].External = false
				}
				return
			}
			cp := CardPull{
				Repo:         rName,
				RepositoryID: repoID,
				Number:       num,
				State:        "open",
				External:     external,
			}
			if threadID != "" {
				cp.ThreadIDs = []string{threadID}
			}
			if p, ok := pulls[pullNumberKey(rName, int64(num))]; ok {
				if p.state != "" {
					cp.State = p.state
				}
				if p.raw != "" && p.raw != "{}" {
					var full github.Pull
					if err := json.Unmarshal([]byte(p.raw), &full); err == nil {
						cp.Title = full.Title
						cp.URL = full.URL
						cp.CI = full.CI
						cp.ReviewDecision = full.ReviewDecision
						cp.ReviewRequests = full.ReviewRequests
						cp.UpdatedAt = full.UpdatedAt
						if full.Draft && cp.State == "open" {
							cp.State = "draft"
						}
						if c.Pull == nil {
							c.Pull = &full
						}
					}
				}
			}
			prIndex[k] = len(c.Pulls)
			c.Pulls = append(c.Pulls, cp)
		}

		threadStatuses := make([]string, 0, len(c.Threads))
		for _, th := range c.Threads {
			threadStatuses = append(threadStatuses, th.Status)
			for _, tp := range tpByThread[th.ID] {
				rName := names[tp.RepositoryID]
				if rName == "" {
					rName = repo
				}
				addPR(rName, tp.RepositoryID, int(tp.Number), th.ID, false)
			}
		}
		for _, num := range u.ClosingPRs {
			repoID := ""
			if u.RepositoryID != nil {
				repoID = *u.RepositoryID
			}
			addPR(repo, repoID, num, "", true)
		}

		// Legacy fallback if task still has pr_number/branch in task_pulls
		if len(c.Pulls) == 0 {
			if key, ok := taskPullKey(c.Repo, t); ok {
				p := pulls[key]
				if p.number > 0 {
					n := int64(p.number)
					c.PRNumber, c.PRState = &n, p.state
					repoID := ""
					if t.RepositoryID != nil {
						repoID = *t.RepositoryID
					}
					addPR(c.Repo, repoID, p.number, "", false)
					if len(c.Pulls) > 0 && p.state != "" {
						c.Pulls[len(c.Pulls)-1].State = p.state
					}
				}
			}
		}
		if len(c.Pulls) > 0 && c.PRState == "" {
			c.PRState = c.Pulls[0].State
		}

		summaries := make([]board.PullSummary, 0, len(c.Pulls))
		for _, cp := range c.Pulls {
			summaries = append(summaries, board.PullSummary{
				Repo:           cp.Repo,
				Number:         cp.Number,
				State:          cp.State,
				CI:             cp.CI,
				ReviewDecision: cp.ReviewDecision,
				ReviewRequests: cp.ReviewRequests,
			})
		}

		res := board.Evaluate(board.CardInput{
			Source:         u.Source,
			Stage:          u.Stage,
			ExternalState:  u.State,
			ExternalReason: u.StateReason,
			PlanAccepted:   c.PlanAccepted,
			Threads:        threadStatuses,
			Pulls:          summaries,
		})
		c.Column = string(res.Column)
		c.Reason = res.Reason

		for _, b := range deps[u.ID] {
			if !board.Finished(localBoardCard(Card{Task: b, Repo: taskRepo(b, names)}, pulls)) {
				c.Blockers = append(c.Blockers, b.ID)
			}
		}
		cards = append(cards, c)
	}
	return cards, nil
}

func localBoardCard(c Card, pulls map[pullKey]cachedPull) board.Card {
	key, _ := taskPullKey(c.Repo, c.Task)
	p := pulls[key]
	return board.Card{Stage: c.Stage, HasPRNumber: c.PRNumber != nil || p.number > 0, PRState: p.state}
}

func (a *Tasks) Columns(ctx context.Context) (map[string]notify.Column, error) {
	cards, err := a.Board(ctx, data.Filter{})
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

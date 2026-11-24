package app

import (
	"context"
	"encoding/json"
	"fmt"

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
}

// Card is the wire representation of a task with its derived board state.
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

func (a *Tasks) Get(ctx context.Context, id string) (data.Task, error) {
	return a.Store.Tasks().Get(ctx, id)
}

func (a *Tasks) List(ctx context.Context, f data.Filter) ([]data.Task, error) {
	return a.Store.Tasks().ListAll(ctx, f)
}

func (a *Tasks) ListPage(ctx context.Context, f data.Filter) ([]data.Task, int, error) {
	return a.Store.Tasks().ListPage(ctx, f)
}

func (a *Tasks) Archive(ctx context.Context, id string) error {
	return a.Store.Tasks().Archive(ctx, id)
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

func (a *Tasks) BoardPage(ctx context.Context, f data.Filter) ([]Card, int, error) {
	tasks, next, err := a.ListPage(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	cards, err := a.cards(ctx, tasks)
	return cards, next, err
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

func (a *Tasks) cards(ctx context.Context, tasks []data.Task) ([]Card, error) {
	cards := make([]Card, 0, len(tasks))
	if len(tasks) == 0 {
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
	names, err := a.repositoryNames(ctx)
	if err != nil {
		return nil, err
	}
	plans, err := a.Store.Plans().ListAll(ctx, data.Filter{})
	if err != nil {
		return nil, err
	}
	withPlan := map[string]bool{}
	for _, p := range plans {
		withPlan[p.TaskID] = true
	}
	deps, err := a.Store.BoardDependencies(ctx)
	if err != nil {
		return nil, err
	}
	pulls, err := a.cachedPulls(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		c := Card{Task: t, Repo: taskRepo(t, names), Threads: byTask[t.ID], HasPlan: withPlan[t.ID], Blockers: []string{}}
		if c.Threads == nil {
			c.Threads = []data.Thread{}
		}
		state := localBoardCard(c, pulls)
		if key, ok := taskPullKey(c.Repo, t); ok {
			p := pulls[key]
			if p.number > 0 {
				n := int64(p.number)
				c.PRNumber, c.PRState = &n, p.state
			}
			if p.raw != "" && p.raw != "{}" {
				var pull github.Pull
				if err := json.Unmarshal([]byte(p.raw), &pull); err != nil {
					return nil, err
				}
				c.Pull = &pull
				state.Pull = &board.PullInfo{CI: pull.CI, ReviewDecision: pull.ReviewDecision, ReviewRequests: pull.ReviewRequests}
			}
		}
		for _, th := range c.Threads {
			state.Threads = append(state.Threads, th.Status)
		}
		for _, b := range deps[t.ID] {
			if !board.Finished(localBoardCard(Card{Task: b, Repo: taskRepo(b, names)}, pulls)) {
				c.Blockers = append(c.Blockers, b.ID)
			}
		}
		c.Column = string(state.Column())
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

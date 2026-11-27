package app

import (
	"context"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/domain/board"
)

// CheckDone accepts a task without pull request or whose pull request is
// merged, using only the local cache of pull requests.
func (a *Tasks) CheckDone(ctx context.Context, t data.Task) error {
	repo, err := a.repoName(ctx, t)
	if err != nil {
		return err
	}
	pulls, err := a.cachedPulls(ctx)
	if err != nil {
		return err
	}
	number := t.PRNumber
	state := ""
	if key, ok := taskPullKey(repo, t); ok {
		p := pulls[key]
		state = p.state
		if number == nil && p.number > 0 {
			n := int64(p.number)
			number = &n
		}
	}
	if number == nil || state == string(board.Done) || state == "merged" {
		return nil
	}
	return data.ErrInvalid.Msgf("the task has an unmerged pull request (#%d); it cannot be marked done", *number).With("reason", "open_pull_request").With("number", *number)
}

// FinishTask is disabled: thread archiving is manual only.
func (a *Tasks) FinishTask(ctx context.Context, taskID string) {}

// Finish is disabled: thread archiving is manual only.
func (a *Tasks) Finish(ctx context.Context) {}

// FinishSoon is disabled: thread archiving is manual only.
func (a *Tasks) FinishSoon(ctx context.Context) {}

// Cards derives the board state of the given tasks.
func (a *Tasks) Cards(ctx context.Context, tasks []data.Task) ([]Card, error) {
	return a.cards(ctx, tasks)
}

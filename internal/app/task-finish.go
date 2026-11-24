package app

import (
	"context"
	"log"
	"time"

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

// FinishTask archives this node's threads of a task that is done or
// cancelled. Archived threads are skipped, so calling it again is safe.
func (a *Tasks) FinishTask(ctx context.Context, taskID string) {
	if a.ArchiveOnFinish == nil || !a.ArchiveOnFinish() {
		return
	}
	threads, err := a.Store.Threads().ListAll(ctx, data.Filter{Fields: map[string]any{"task_id": taskID}})
	if err != nil {
		log.Printf("app: finish task %s: %v", taskID, err)
		return
	}
	for _, t := range threads {
		if t.NodeID != a.Node {
			continue
		}
		if err := a.Threads.Archive(ctx, t.ID, ArchiveOptions{}); err != nil {
			log.Printf("app: archive thread %s: %v", t.ID, err)
		}
	}
}

// Finish archives the threads of every done or cancelled task. It is
// idempotent and runs when the pull request cache or a task changes.
func (a *Tasks) Finish(ctx context.Context) {
	if a.ArchiveOnFinish == nil || !a.ArchiveOnFinish() {
		return
	}
	a.finishing.Lock()
	defer a.finishing.Unlock()
	cards, err := a.Board(ctx, data.Filter{})
	if err != nil {
		log.Printf("app: finish tasks: %v", err)
		return
	}
	for _, c := range cards {
		if c.Column == string(board.Done) || c.Column == string(board.Cancelled) {
			a.FinishTask(ctx, c.ID)
		}
	}
}

// finishDelay is how long FinishSoon waits for changes to settle.
const finishDelay = 2 * time.Second

// FinishSoon runs Finish once the changes of tasks and threads settle, so a
// burst of writes costs one pass over the board.
func (a *Tasks) FinishSoon(ctx context.Context) {
	if a.ArchiveOnFinish == nil || !a.ArchiveOnFinish() {
		return
	}
	a.finishSoon.call(finishDelay, func() {
		if ctx.Err() == nil {
			a.Finish(ctx)
		}
	})
}

// Cards derives the board state of the given tasks.
func (a *Tasks) Cards(ctx context.Context, tasks []data.Task) ([]Card, error) {
	return a.cards(ctx, tasks)
}

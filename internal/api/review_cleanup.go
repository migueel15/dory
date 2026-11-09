package api

import (
	"context"
	"log"
	"time"

	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// RunReviewCleanup watches the pull requests that have a review thread. When
// one is merged or closed, it removes the thread's worktree through T3 on the
// machine that runs it (git refuses if it has uncommitted changes) and
// archives the link, so the PR no longer shows the chat. A machine that cannot
// be reached, or a removal that fails, is retried on the next pass.
func RunReviewCleanup(ctx context.Context, svc service.Service, monitor *t3.Monitor, gh *github.Client) {
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			cleanReviews(ctx, svc, monitor, gh)
		}
	}
}

func cleanReviews(ctx context.Context, svc service.Service, monitor *t3.Monitor, gh *github.Client) {
	links, err := svc.IssueThreads().List(ctx, service.Filter{Fields: map[string]any{"kind": "review"}, Limit: 1000})
	if err != nil {
		log.Printf("review cleanup: %v", err)
		return
	}
	for _, link := range links {
		detail, err := gh.Detail(ctx, link.Repo, "pulls", link.Number)
		if err != nil || detail.Pull == nil {
			continue
		}
		state := detail.Pull.State
		if state != "MERGED" && state != "CLOSED" {
			continue
		}
		c, err := monitor.ThreadClient(ctx, link.ThreadID)
		if err != nil {
			c, err = monitor.InstanceClient(ctx, link.InstanceID)
		}
		if err == nil {
			var root, worktree string
			if root, worktree, err = c.ThreadWorktree(ctx, link.ThreadID); err == nil && worktree != "" {
				err = c.RemoveWorktree(ctx, root, worktree)
			}
		}
		if err != nil {
			log.Printf("review cleanup %s#%d: %v", link.Repo, link.Number, err)
			continue
		}
		monitor.Refresh(link.InstanceID)
		if err := svc.IssueThreads().Archive(ctx, "review-cleanup", link.ID); err != nil {
			log.Printf("review cleanup archive %s: %v", link.ID, err)
		}
	}
}

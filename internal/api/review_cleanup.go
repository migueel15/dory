package api

import (
	"context"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// RunReviewCleanup watches the pull requests that have a review thread. When
// one is merged or closed, it removes the thread's worktree through T3 on the
// machine that runs it (git refuses if it has uncommitted changes) and
// keeps the link so the PR continues to show the chat. A machine that cannot
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
			cleanImplementations(ctx, svc, monitor, gh)
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
	}
}

var pullRefPattern = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/pull/(\d+)/?$`)

// cleanImplementations removes, once its pull request is merged or closed, the
// worktree Dory created for an implementation thread (the item keeps a
// "worktree:<path>" ref). Git refuses when it has uncommitted changes, and the
// ref stays so the next pass retries. On success the ref is dropped.
func cleanImplementations(ctx context.Context, svc service.Service, monitor *t3.Monitor, gh *github.Client) {
	for _, status := range []string{"in_progress", "validating", "done"} {
		items, err := svc.Items().List(ctx, service.Filter{Fields: map[string]any{"status": status}, Limit: 1000})
		if err != nil {
			log.Printf("implementation cleanup: %v", err)
			return
		}
		for _, item := range items {
			cleanImplementation(ctx, svc, monitor, gh, item)
		}
	}
}

func cleanImplementation(ctx context.Context, svc service.Service, monitor *t3.Monitor, gh *github.Client, item service.Item) {
	worktree, pr := "", ""
	for _, ref := range item.Refs {
		if strings.HasPrefix(ref, worktreeRefPrefix) {
			worktree = strings.TrimPrefix(ref, worktreeRefPrefix)
		} else if pullRefPattern.MatchString(ref) {
			pr = ref
		}
	}
	if worktree == "" || pr == "" {
		return
	}
	m := pullRefPattern.FindStringSubmatch(pr)
	number, _ := strconv.Atoi(m[2])
	detail, err := gh.Detail(ctx, m[1], "pulls", number)
	if err != nil || detail.Pull == nil || (detail.Pull.State != "MERGED" && detail.Pull.State != "CLOSED") {
		return
	}
	threads, err := svc.ItemThreads().List(ctx, service.Filter{Fields: map[string]any{"item_id": item.ID}, Limit: 20})
	if err != nil || len(threads) == 0 {
		return
	}
	link := threads[0]
	c, err := monitor.ThreadClient(ctx, link.ThreadID)
	if err != nil {
		c, err = monitor.InstanceClient(ctx, link.InstanceID)
	}
	if err == nil {
		var root string
		if root, _, err = c.ThreadWorktree(ctx, link.ThreadID); err == nil {
			err = c.RemoveWorktree(ctx, root, worktree)
		}
	}
	if err != nil {
		log.Printf("implementation cleanup %s: %v", item.ID, err)
		return
	}
	monitor.Refresh(link.InstanceID)
	kept := item.Refs[:0:0]
	for _, ref := range item.Refs {
		if ref != worktreeRefPrefix+worktree {
			kept = append(kept, ref)
		}
	}
	item.Refs = kept
	if _, err := svc.Items().Put(ctx, "worktree-cleanup", item); err != nil {
		log.Printf("implementation cleanup save %s: %v", item.ID, err)
	}
}

package mcp

import (
	"context"
	"strings"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func addPullTools(srv *sdk.Server, s *Deps) {
	store := s.Store

	resolveRepo := func(ctx context.Context, repoArg, thID string) (data.Repository, string, error) {
		repos, err := store.Repositories().ListAll(ctx, data.Filter{})
		if err != nil {
			return data.Repository{}, "", err
		}
		if repoArg != "" {
			for _, r := range repos {
				if r.ID == repoArg {
					name, _ := github.ParseRemote(r.RemoteURL)
					return r, name, nil
				}
				if name, ok := github.ParseRemote(r.RemoteURL); ok && strings.EqualFold(name, repoArg) {
					return r, name, nil
				}
			}
			return data.Repository{}, "", data.ErrNotFound.Msgf("repository not found: %s", repoArg)
		}
		if thID != "" {
			if th, err := store.Threads().Get(ctx, thID); err == nil && th.ProjectID != nil {
				for _, r := range repos {
					if r.ProjectID == *th.ProjectID {
						name, _ := github.ParseRemote(r.RemoteURL)
						return r, name, nil
					}
				}
			}
		}
		return data.Repository{}, "", apperr.New(apperr.Invalid, "repo is required when it cannot be inferred from the thread")
	}

	add(srv, "list_pulls", "List pull requests linked to the current thread (or all thread_pulls).", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id,omitempty"`
	}) (any, error) {
		thID, _ := threadID(ctx, in.ThreadID)
		if thID != "" {
			return store.ThreadPulls().ListByThread(ctx, thID)
		}
		return store.ThreadPulls().ListAll(ctx, data.Filter{})
	})

	add(srv, "get_pull", "Get pull request checks and status from GitHub.", func(ctx context.Context, in struct {
		Repo   string `json:"repo,omitempty"`
		Number int    `json:"number"`
	}) (any, error) {
		if s.GitHub == nil {
			return nil, github.ErrUnconfigured
		}
		thID, _ := threadID(ctx, "")
		_, repoName, err := resolveRepo(ctx, in.Repo, thID)
		if err != nil {
			return nil, err
		}
		checks, err := s.GitHub.PullChecks(ctx, repoName, in.Number)
		if err != nil {
			return nil, err
		}
		return map[string]any{"repo": repoName, "number": in.Number, "checks": checks}, nil
	})

	add(srv, "create_pull", "Create a GitHub pull request and link it to the current thread.", func(ctx context.Context, in struct {
		Repo  string `json:"repo,omitempty" jsonschema:"owner/repo; inferred from thread project if omitted"`
		Title string `json:"title"`
		Body  string `json:"body,omitempty"`
		Head  string `json:"head,omitempty" jsonschema:"Head branch; inferred from thread branch if omitted"`
		Base  string `json:"base,omitempty" jsonschema:"Base branch; inferred from repository default branch if omitted"`
		Draft bool   `json:"draft,omitempty"`
	}) (any, error) {
		if s.GitHub == nil {
			return nil, github.ErrUnconfigured
		}
		thID, _ := threadID(ctx, "")
		repo, repoName, err := resolveRepo(ctx, in.Repo, thID)
		if err != nil {
			return nil, err
		}
		head := in.Head
		if head == "" && thID != "" {
			if th, err := store.Threads().Get(ctx, thID); err == nil {
				head = th.Branch
			}
		}
		base := in.Base
		if base == "" {
			base = repo.DefaultBranch
		}
		if base == "" {
			base = "main"
		}
		pr, err := s.GitHub.CreatePull(ctx, repoName, github.PullCreateInput{
			Title: in.Title,
			Body:  in.Body,
			Head:  head,
			Base:  base,
			Draft: in.Draft,
		})
		if err != nil {
			return nil, err
		}
		if thID != "" {
			_, _ = store.ThreadPulls().Link(ctx, thID, repo.ID, int64(pr.Number), head, "mcp")
		}
		if s.Notify != nil {
			s.Notify("board", "", nil)
		}
		return pr, nil
	})

	add(srv, "link_pull", "Link an existing pull request to a thread (this one by default).", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id,omitempty"`
		Repo     string `json:"repo,omitempty"`
		Number   int64  `json:"number"`
		Branch   string `json:"branch,omitempty"`
	}) (any, error) {
		thID, err := threadID(ctx, in.ThreadID)
		if err != nil {
			return nil, err
		}
		repo, _, err := resolveRepo(ctx, in.Repo, thID)
		if err != nil {
			return nil, err
		}
		tp, err := store.ThreadPulls().Link(ctx, thID, repo.ID, in.Number, in.Branch, "mcp")
		if err != nil {
			return nil, err
		}
		if s.Notify != nil {
			s.Notify("board", "", nil)
		}
		return tp, nil
	})

	add(srv, "unlink_pull", "Unlink a pull request from a thread (this one by default).", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id,omitempty"`
		Repo     string `json:"repo,omitempty"`
		Number   int64  `json:"number"`
	}) (any, error) {
		thID, err := threadID(ctx, in.ThreadID)
		if err != nil {
			return nil, err
		}
		repo, _, err := resolveRepo(ctx, in.Repo, thID)
		if err != nil {
			return nil, err
		}
		if err := store.ThreadPulls().Unlink(ctx, thID, repo.ID, in.Number); err != nil {
			return nil, err
		}
		if s.Notify != nil {
			s.Notify("board", "", nil)
		}
		return map[string]any{"unlinked": true}, nil
	})

	add(srv, "update_pull", "Update a GitHub pull request's title, body, draft state, assignees or reviewers.", func(ctx context.Context, in struct {
		Repo      string    `json:"repo,omitempty"`
		Number    int       `json:"number"`
		Title     *string   `json:"title,omitempty"`
		Body      *string   `json:"body,omitempty"`
		State     *string   `json:"state,omitempty"`
		Draft     *bool     `json:"draft,omitempty"`
		Assignees *[]string `json:"assignees,omitempty"`
		Reviewers *[]string `json:"reviewers,omitempty"`
	}) (any, error) {
		if s.GitHub == nil {
			return nil, github.ErrUnconfigured
		}
		thID, _ := threadID(ctx, "")
		_, repoName, err := resolveRepo(ctx, in.Repo, thID)
		if err != nil {
			return nil, err
		}
		pr, err := s.GitHub.UpdatePull(ctx, repoName, in.Number, github.PullUpdateInput{
			Title:     in.Title,
			Body:      in.Body,
			State:     in.State,
			Draft:     in.Draft,
			Assignees: in.Assignees,
			Reviewers: in.Reviewers,
		})
		if err != nil {
			return nil, err
		}
		if s.Notify != nil {
			s.Notify("board", "", nil)
		}
		return pr, nil
	})

	add(srv, "review_pull", "Submit a review on a GitHub pull request. Rejects APPROVE and REQUEST_CHANGES on your own PRs.", func(ctx context.Context, in struct {
		Repo     string                      `json:"repo,omitempty"`
		Number   int                         `json:"number"`
		Event    string                      `json:"event" jsonschema:"APPROVE, REQUEST_CHANGES or COMMENT"`
		Body     string                      `json:"body,omitempty"`
		Comments []github.ReviewCommentInput `json:"comments,omitempty"`
	}) (any, error) {
		if s.GitHub == nil {
			return nil, github.ErrUnconfigured
		}
		thID, _ := threadID(ctx, "")
		_, repoName, err := resolveRepo(ctx, in.Repo, thID)
		if err != nil {
			return nil, err
		}
		if err := s.GitHub.ReviewPull(ctx, repoName, in.Number, github.PullReviewInput{
			Event:    in.Event,
			Body:     in.Body,
			Comments: in.Comments,
		}); err != nil {
			return nil, err
		}
		if s.Notify != nil {
			s.Notify("board", "", nil)
		}
		return map[string]any{"reviewed": true}, nil
	})

	add(srv, "merge_pull", "Merge a GitHub pull request.", func(ctx context.Context, in struct {
		Repo         string `json:"repo,omitempty"`
		Number       int    `json:"number"`
		Method       string `json:"method,omitempty" jsonschema:"merge, squash or rebase"`
		CommitTitle  string `json:"commit_title,omitempty"`
		DeleteBranch bool   `json:"delete_branch,omitempty"`
	}) (any, error) {
		if s.GitHub == nil {
			return nil, github.ErrUnconfigured
		}
		thID, _ := threadID(ctx, "")
		_, repoName, err := resolveRepo(ctx, in.Repo, thID)
		if err != nil {
			return nil, err
		}
		if err := s.GitHub.MergePull(ctx, repoName, in.Number, github.PullMergeInput{
			Method:       in.Method,
			CommitTitle:  in.CommitTitle,
			DeleteBranch: in.DeleteBranch,
		}); err != nil {
			return nil, err
		}
		if s.Notify != nil {
			s.Notify("board", "", nil)
		}
		return map[string]any{"merged": true}, nil
	})
}

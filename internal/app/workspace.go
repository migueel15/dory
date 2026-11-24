package app

import (
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
)

var ErrRepositoryNotCloned = apperr.New(apperr.RepositoryNotClone, "the repository is not cloned on this node")

// RepositoryCheckout resolves an explicit repository or the project's first one.
// Projects without repositories can start threads in a scratch directory.
func RepositoryCheckout(ctx context.Context, store RepositoryStore, project *string, repo string) (string, string, error) {
	if repo == "" && project != nil {
		var err error
		repo, err = store.ProjectRepository(ctx, *project)
		if err != nil {
			return "", "", err
		}
	}
	if repo == "" {
		return "", "", nil
	}
	path, err := store.RepositoryPath(ctx, repo)
	if errors.Is(err, data.ErrNotFound) || (err == nil && path == "") {
		err = ErrRepositoryNotCloned
	}
	return repo, path, err
}

type preparedWorkspace struct {
	dir, base            string
	repo, branch         string
	added, createdBranch bool
}

func (s *Threads) prepareWorkspace(ctx context.Context, t *data.Thread, in CreateThread, repo string, defaults ThreadSettings) (preparedWorkspace, error) {
	out := preparedWorkspace{dir: repo, base: in.BaseBranch, repo: repo}
	if in.ParentID != nil && in.Inherit != nil && in.Inherit.ID == *in.ParentID && in.Inherit.NodeID == s.Node &&
		sameProject(t.ProjectID, in.Inherit.ProjectID) && in.RepositoryID == "" && in.WorktreePath == "" && in.Worktree == nil && in.Branch == "" && in.BaseBranch == "" {
		parent, err := s.Store.Thread(ctx, *in.ParentID)
		if err != nil {
			return out, err
		}
		if parent.NodeID == s.Node && sameProject(t.ProjectID, parent.ProjectID) {
			out.dir, err = s.Runtime.Dir(ctx, parent.ID)
			t.Branch = parent.Branch
			out.base = s.worktreeBase(ctx, repo, out.dir)
			return out, err
		}
	}
	if in.WorktreePath != "" && repo != "" {
		for _, wt := range s.Workspace.Worktrees(ctx, repo) {
			if wt.Path == in.WorktreePath {
				out.dir, t.Branch = wt.Path, wt.Branch
				out.base = s.worktreeBase(ctx, repo, out.dir)
				return out, nil
			}
		}
	}
	if in.WorktreePath != "" {
		return out, apperr.New(apperr.UnknownWorktree, "the worktree does not belong to the project's repository")
	}
	newWorktree := repo != "" && in.Branch == "" && defaults.DefaultWorkspace == "worktree"
	if in.Worktree != nil {
		newWorktree = *in.Worktree
	}
	if !newWorktree {
		if t.Branch != "" && repo != "" {
			if err := s.Workspace.SwitchBranch(ctx, repo, t.Branch); err != nil {
				if errors.Is(err, apperr.New(apperr.BranchSwitchFailed, "")) {
					return out, err
				}
				return out, apperr.Wrap(apperr.BranchSwitchFailed, err, "could not switch checkout branch").With("branch", t.Branch)
			}
		}
		return out, nil
	}
	if repo == "" {
		return out, data.ErrInvalid.Msgf("worktree needs a repository path")
	}
	if t.Branch == "" {
		if s.Namer != nil {
			t.Branch = s.Namer.Name(ctx, defaults.BranchNaming, t.Title, in.Text)
		}
		if t.Branch == "" || s.Workspace.BranchExists(ctx, repo, t.Branch) {
			t.Branch = "dory/" + strings.ToLower(rand.Text()[:8])
		}
	}
	root := defaults.WorktreeRoot
	if root == "" {
		root = filepath.Join(s.DataDir, "worktrees")
	}
	out.dir, out.branch = filepath.Join(root, t.ID), t.Branch
	var err error
	out.createdBranch, err = s.Workspace.AddWorktree(ctx, repo, out.dir, t.Branch, out.base)
	out.added = err == nil
	return out, err
}

func (s *Threads) worktreeBase(ctx context.Context, repo, dir string) string {
	if use, err := s.Runtime.WorktreeUse(ctx, dir, ""); err == nil && use.BaseBranch != "" {
		return use.BaseBranch
	}
	if repo != "" {
		return s.Workspace.DefaultBranch(ctx, repo)
	}
	return ""
}

func (s *Threads) undoWorkspace(ctx context.Context, prepared preparedWorkspace) error {
	if !prepared.added {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.Workspace.RemoveWorktree(ctx, prepared.dir, true); err != nil {
		return err
	}
	if prepared.createdBranch {
		return s.Workspace.DeleteBranch(ctx, prepared.repo, prepared.branch)
	}
	return nil
}

func sameProject(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

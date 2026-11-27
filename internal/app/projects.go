package app

import (
	"context"
	"errors"
	"strings"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/oplog"
)

// Projects finds projects and links tasks and repositories to them.
type Projects struct {
	Store *data.Store
}

// FindInput says how session_start finds a project: by id or slug, by git
// remote, or by a local checkout path, in that order.
type FindInput struct {
	Project   string
	RemoteURL string
	Path      string
}

// List is every project.
func (a *Projects) List(ctx context.Context) ([]data.Project, error) {
	return a.Store.Projects().ListAll(ctx, data.Filter{})
}

// Find returns the project of the input among projects, or nil if none matches.
func (a *Projects) Find(ctx context.Context, projects []data.Project, in FindInput) (*data.Project, error) {
	for i, p := range projects {
		if in.Project != "" && (p.ID == in.Project || p.Slug == in.Project) {
			return &projects[i], nil
		}
	}
	remote := in.RemoteURL
	if remote == "" && in.Path != "" {
		paths, err := a.Store.RepositoryPaths(ctx)
		if err != nil {
			return nil, err
		}
		for id, path := range paths {
			if path == in.Path || strings.HasPrefix(in.Path, path+"/") {
				if r, err := a.Store.Repositories().Get(ctx, id); err == nil {
					remote = r.RemoteURL
				}
			}
		}
	}
	if remote == "" {
		return nil, nil
	}
	r, err := a.Store.Repositories().Get(ctx, oplog.RepositoryID(remote))
	if errors.Is(err, data.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for i, p := range projects {
		if p.ID == r.ProjectID {
			return &projects[i], nil
		}
	}
	return nil, nil
}

// Repositories are the repositories of a project.
func (a *Projects) Repositories(ctx context.Context, projectID string) ([]data.Repository, error) {
	return a.Store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": projectID}})
}

// SetTaskProject moves a task to a project (id or slug), or out of it with an
// empty project. A project with one repository gives it to the task.
func (a *Projects) SetTaskProject(ctx context.Context, taskID, project string) (data.Task, error) {
	store := a.Store
	t, err := store.Tasks().Get(ctx, taskID)
	if err != nil {
		return t, err
	}
	var next *string
	if project != "" {
		p, err := store.Projects().Get(ctx, project)
		if errors.Is(err, data.ErrNotFound) {
			projects, lerr := a.List(ctx)
			if lerr != nil {
				return t, lerr
			}
			for i := range projects {
				if projects[i].Slug == project {
					p, err = projects[i], nil
					break
				}
			}
		}
		if err != nil {
			return t, err
		}
		if p.ID == "" {
			return t, data.ErrInvalid.Msgf("project %s does not exist", project)
		}
		next = &p.ID
	}
	if (t.ProjectID == nil) == (next == nil) && (next == nil || *t.ProjectID == *next) {
		return t, nil
	}
	t.ProjectID, t.RepositoryID = next, nil
	if next != nil {
		if repos, _ := a.Repositories(ctx, *next); len(repos) == 1 {
			t.RepositoryID = &repos[0].ID
		}
	}
	return store.Tasks().PatchIf(ctx, t.ID, t.UpdatedAt, data.Fields{"project_id": t.ProjectID, "repository_id": t.RepositoryID})
}

// FromRepository registers a repository, creating its project if needed.
func (a *Projects) FromRepository(ctx context.Context, name, slug, description, remote, branch string) (data.Repository, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return data.Repository{}, data.ErrInvalid.Msgf("remote_url required")
	}
	if repo, err := a.Store.Repositories().Get(ctx, oplog.RepositoryID(remote)); err == nil {
		if _, err = a.Store.Projects().Get(ctx, repo.ProjectID); err == nil {
			return repo, nil
		}
	}
	projectID := oplog.ProjectID(remote)
	p, err := a.Store.Projects().Get(ctx, projectID)
	if errors.Is(err, data.ErrNotFound) {
		p, err = a.Store.Projects().Create(ctx, data.Project{ID: projectID, Name: name, Slug: slug, Description: description})
		if errors.Is(err, data.ErrConflict) {
			p, err = a.Store.Projects().Get(ctx, projectID)
		}
	}
	if err != nil {
		return data.Repository{}, err
	}
	if branch == "" {
		branch = "main"
	}
	return a.Store.Repositories().Upsert(ctx, data.Repository{ProjectID: p.ID, RemoteURL: remote, DefaultBranch: branch})
}

// Checkout is the repository and local path of the checkout of a project.
func (a *Projects) Checkout(ctx context.Context, projectID string) (repositoryID, path string, err error) {
	return RepositoryCheckout(ctx, a.Store, &projectID, "")
}

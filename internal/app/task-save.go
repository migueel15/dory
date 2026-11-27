package app

import (
	"context"
	"slices"

	"github.com/migueel15/dory/internal/data"
)

// SaveInput is what an agent sends to create or change a task; a zero field
// leaves the task as it is.
type SaveInput struct {
	ID           string
	ProjectID    string
	RepositoryID string
	Title        string
	Description  string
	IssueNumber  int64
	PRNumber     int64
	Stage        string
	DependsOn    []string
}

// ListInput filters the task index.
type ListInput struct {
	ProjectID   string
	IssueNumber int64
	Open        bool
}

// ListCards is the board cards of a project or issue; Open leaves out done
// and cancelled tasks.
func (a *Tasks) ListCards(ctx context.Context, in ListInput) ([]Card, error) {
	cards, err := a.BoardMulti(ctx, TaskFilter{ProjectID: in.ProjectID, Open: in.Open})
	if err != nil {
		return nil, err
	}
	if in.IssueNumber != 0 {
		cards = slices.DeleteFunc(cards, func(c Card) bool {
			n := c.Number
			if c.Source != "github" {
				n = c.IssueNumber
			}
			return n == nil || *n != in.IssueNumber
		})
	}
	return cards, nil
}

// Save creates a task or changes the given fields of one. A task of a
// project with one repository takes that repository.
func (a *Tasks) Save(ctx context.Context, in SaveInput) (data.Task, error) {
	var t data.Task
	if in.ID != "" {
		var err error
		if t, err = a.Adopt(ctx, in.ID); err != nil {
			return t, err
		}
	} else if in.Title == "" {
		return t, data.ErrInvalid.Msgf("title is required")
	}
	fields := data.Fields{}
	if in.ProjectID != "" {
		t.ProjectID = &in.ProjectID
		fields["project_id"] = in.ProjectID
	}
	if in.RepositoryID != "" {
		t.RepositoryID = &in.RepositoryID
		fields["repository_id"] = in.RepositoryID
	}
	if in.Title != "" {
		t.Title = in.Title
		fields["title"] = in.Title
	}
	if in.Description != "" {
		t.Description = in.Description
		fields["description"] = in.Description
	}
	if in.IssueNumber != 0 {
		t.IssueNumber = &in.IssueNumber
		fields["issue_number"] = in.IssueNumber
	}
	if in.PRNumber != 0 {
		t.PRNumber = &in.PRNumber
		fields["pr_number"] = in.PRNumber
	}
	if in.Stage != "" {
		t.Stage = in.Stage
		fields["stage"] = in.Stage
	}
	var expected *string
	if t.RepositoryID == nil && t.ProjectID != nil {
		if repos, _ := a.Store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": *t.ProjectID}}); len(repos) == 1 {
			t.RepositoryID = &repos[0].ID
			fields["repository_id"] = repos[0].ID
			expected = &t.UpdatedAt
		}
	}
	var err error
	if t.ID == "" {
		// The issue of a new task is linked once the task exists.
		issue := t.IssueNumber
		t.IssueNumber = nil
		if t, err = a.Create(ctx, t); err == nil && issue != nil {
			t, err = a.Patch(ctx, t.ID, nil, data.Fields{"issue_number": *issue})
		}
	} else {
		t, err = a.Patch(ctx, t.ID, expected, fields)
	}
	if err != nil {
		return t, err
	}
	for _, dep := range in.DependsOn {
		if _, err = a.AddDependency(ctx, t.ID, dep); err != nil {
			return t, err
		}
	}
	return t, nil
}

package data

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// LinkThread links a thread to an existing task of its project. Linking to the
// same task again changes nothing; moving it from another task needs replace.
// The link is a field of the replicated thread, so it works for threads of
// any node.
func (s *Store) LinkThread(ctx context.Context, threadID, taskID string, replace bool) (Thread, error) {
	th, err := s.Threads().Get(ctx, threadID)
	if err != nil {
		return th, err
	}
	if taskID == "" {
		return th, ErrInvalid.Msgf("task_id is required")
	}
	task, err := s.Tasks().Get(ctx, taskID)
	if errors.Is(err, ErrNotFound) {
		return th, ErrInvalid.Msgf("task %s does not exist or is archived", taskID)
	} else if err != nil {
		return th, err
	}
	if th.ProjectID != nil && (task.ProjectID == nil || *task.ProjectID != *th.ProjectID) {
		return th, ErrInvalid.Msgf("task %s belongs to another project", taskID)
	}
	if th.TaskID != nil {
		if *th.TaskID == taskID {
			return th, nil
		}
		if !replace {
			return th, ErrInvalid.Msgf("thread is linked to task %s; pass replace to move it", *th.TaskID)
		}
	}
	return s.Threads().PatchIf(ctx, threadID, th.UpdatedAt, Fields{"task_id": taskID, "task_linked_at": s.now().UTC().Format(time.RFC3339Nano)})
}

// UnlinkThread removes the task of a thread.
func (s *Store) UnlinkThread(ctx context.Context, threadID string) (Thread, error) {
	th, err := s.Threads().Get(ctx, threadID)
	if err != nil || th.TaskID == nil {
		return th, err
	}
	return s.Threads().PatchIf(ctx, threadID, th.UpdatedAt, Fields{"task_id": nil, "task_linked_at": nil})
}

// AddDependency marks that a task must wait for another. It refuses a task
// that depends on itself and any dependency that would close a cycle.
func (s *Store) AddDependency(ctx context.Context, taskID, dependsOnID string) (Dependency, error) {
	if taskID == "" || dependsOnID == "" {
		return Dependency{}, ErrInvalid.Msgf("task_id and depends_on_id are required")
	}
	if taskID == dependsOnID {
		return Dependency{}, ErrInvalid.Msgf("a task cannot depend on itself")
	}
	for _, id := range []string{taskID, dependsOnID} {
		if _, err := s.Tasks().Get(ctx, id); err != nil {
			return Dependency{}, fmt.Errorf("task %s: %w", id, err)
		}
	}
	// A cycle exists if taskID is reachable from dependsOnID.
	seen := map[string]bool{}
	next := []string{dependsOnID}
	for len(next) > 0 {
		id := next[0]
		next = next[1:]
		if id == taskID {
			return Dependency{}, ErrInvalid.Msgf("the dependency would create a cycle")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		deps, err := s.Dependencies().ListAll(ctx, Filter{Fields: map[string]any{"task_id": id}})
		if err != nil {
			return Dependency{}, err
		}
		for _, d := range deps {
			next = append(next, d.DependsOnID)
		}
	}
	return s.Dependencies().Upsert(ctx, Dependency{TaskID: taskID, DependsOnID: dependsOnID})
}

// RemoveDependency archives a dependency, which replicates like any change.
func (s *Store) RemoveDependency(ctx context.Context, taskID, dependsOnID string) error {
	return s.Dependencies().Archive(ctx, taskID+"/"+dependsOnID)
}

// BoardDependencies returns every active dependency with the fields needed
// to resolve its blocker, including blockers outside the board's filter.
func (s *Store) BoardDependencies(ctx context.Context) (map[string][]Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.task_id, t.id, t.stage, t.repository_id, t.branch, t.pr_number
		FROM task_dependencies d JOIN tasks t ON t.id = d.depends_on_id
		WHERE d.archived_at IS NULL AND t.archived_at IS NULL
		ORDER BY d.created_at, d.task_id, d.depends_on_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byTask := map[string][]Task{}
	for rows.Next() {
		var id string
		var t Task
		if err := rows.Scan(&id, &t.ID, &t.Stage, &t.RepositoryID, &t.Branch, &t.PRNumber); err != nil {
			return nil, err
		}
		byTask[id] = append(byTask[id], t)
	}
	return byTask, rows.Err()
}

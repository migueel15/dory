package data

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"time"

	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
)

// ExternalTaskID returns the deterministic UUIDv5 for a GitHub issue task.
func ExternalTaskID(repositoryID string, number int64) string {
	return ExternalTaskIDForSource("github", repositoryID, number)
}

// ExternalTaskIDForSource returns the deterministic UUIDv5 for an external task.
func ExternalTaskIDForSource(source, repositoryID string, number int64) string {
	if source == "" {
		source = "github"
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("dory-task:%s:%s#%d", source, repositoryID, number)))
	sum[6] = sum[6]&0x0f | 0x50 // version 5
	sum[8] = sum[8]&0x3f | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// EnsureExternalTask creates or returns the replicated task row for an external issue.
func (s *Store) EnsureExternalTask(ctx context.Context, projectID *string, repositoryID string, number int64) (Task, error) {
	if repositoryID == "" || number <= 0 {
		return Task{}, ErrInvalid.Msgf("repository_id and positive number required")
	}
	id := ExternalTaskID(repositoryID, number)
	var out Task
	spec, _ := schema.Lookup("task")
	err := s.log.Tx(ctx, func(tx *oplog.Tx) error {
		existing, err := s.Tasks().get(ctx, tx, id, true)
		if err == nil {
			if existing.ArchivedAt != nil {
				out, err = s.Tasks().patchTx(ctx, tx, spec, id, nil, Fields{"archived_at": nil})
				return err
			}
			out = existing
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		fields := Fields{
			"source":        "github",
			"project_id":    projectID,
			"repository_id": repositoryID,
			"number":        number,
			"title":         "",
			"description":   "",
			"stage":         "",
			"branch":        "",
			"position":      "",
			"created_at":    now,
			"updated_at":    now,
		}
		out, err = s.Tasks().write(ctx, tx, id, fields)
		return err
	})
	return writeResult(out, err)
}

// MoveStage updates a task's manual stage. Moving to inbox clears accepted_at
// on any accepted plan for the task in the same transaction. Moving an internal
// task to done fails if it has any open pull request.
func (s *Store) MoveStage(ctx context.Context, taskID, stage string) (Task, error) {
	var out Task
	taskSpec, _ := schema.Lookup("task")
	planSpec, _ := schema.Lookup("plan")
	err := s.log.Tx(ctx, func(tx *oplog.Tx) error {
		task, err := s.Tasks().get(ctx, tx, taskID, false)
		if err != nil {
			return err
		}
		isExternal := task.Source != "" && task.Source != "internal"
		if isExternal {
			if stage != "" && stage != "inbox" && stage != "ready" {
				return ErrInvalid.Msgf("external task stage must be empty, inbox or ready")
			}
		} else {
			if stage != "inbox" && stage != "ready" && stage != "done" && stage != "cancelled" {
				return ErrInvalid.Msgf("invalid task stage %q", stage)
			}
		}
		if !isExternal && stage == "done" {
			rows, err := tx.QueryContext(ctx, `
				SELECT COUNT(*)
				FROM thread_pulls tp
				JOIN threads th ON th.id = tp.thread_id AND th.archived_at IS NULL
				JOIN repositories r ON r.id = tp.repository_id AND r.archived_at IS NULL
				JOIN task_pulls p ON p.number = tp.number AND p.state IN ('open', 'draft')
				WHERE th.task_id = ? AND tp.archived_at IS NULL
			`, taskID)
			if err != nil {
				return err
			}
			var openPulls int
			if rows.Next() {
				if err := rows.Scan(&openPulls); err != nil {
					rows.Close()
					return err
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			if openPulls > 0 {
				return ErrInvalid.Msgf("cannot mark task done while it has open pull requests")
			}
		}
		if stage == "inbox" {
			plans, err := s.Plans().list(ctx, tx, Filter{Fields: map[string]any{"task_id": taskID}, Limit: 100}, false)
			if err != nil {
				return err
			}
			for _, p := range plans {
				if p.AcceptedAt != nil {
					if _, err := s.Plans().patchTx(ctx, tx, planSpec, p.ID, nil, Fields{"accepted_at": nil}); err != nil {
						return err
					}
				}
			}
		}
		out, err = s.Tasks().patchTx(ctx, tx, taskSpec, taskID, nil, Fields{"stage": stage})
		return err
	})
	return writeResult(out, err)
}

// ThreadPullStore extends Resource[ThreadPull] with link/unlink helpers.
type ThreadPullStore struct {
	Resource[ThreadPull]
}

// Link creates or unarchives a thread_pull row.
func (r ThreadPullStore) Link(ctx context.Context, threadID, repositoryID string, number int64, branch, linkedBy string) (ThreadPull, error) {
	if linkedBy == "" {
		linkedBy = "manual"
	}
	return r.Upsert(ctx, ThreadPull{
		ThreadID:     threadID,
		RepositoryID: repositoryID,
		Number:       number,
		Branch:       branch,
		LinkedBy:     linkedBy,
	})
}

// Unlink archives a thread_pull row so branch detection will not recreate it.
func (r ThreadPullStore) Unlink(ctx context.Context, threadID, repositoryID string, number int64) error {
	id := fmt.Sprintf("%s/%s/%d", threadID, repositoryID, number)
	_, err := r.Patch(ctx, id, Fields{"archived_at": r.s.now().UTC().Format(time.RFC3339Nano)})
	return err
}

// ListByThread returns all active thread_pull rows for a thread.
func (r ThreadPullStore) ListByThread(ctx context.Context, threadID string) ([]ThreadPull, error) {
	return r.ListAll(ctx, Filter{Fields: map[string]any{"thread_id": threadID}})
}

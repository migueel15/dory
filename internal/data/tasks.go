package data

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
)

// AdoptedTaskID is the id of the task that adopts an external item. It comes
// from the item's key, so adopting it on two nodes gives the same task.
func AdoptedTaskID(source, externalID string) string {
	sum := sha1.Sum([]byte("dory-task:" + source + ":" + externalID))
	sum[6] = sum[6]&0x0f | 0x50 // version 5
	sum[8] = sum[8]&0x3f | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// ExternalRef is an item of an external provider: its key and the data a task
// keeps when it adopts the item.
type ExternalRef struct {
	Source       string
	ExternalID   string
	ProjectID    *string
	RepositoryID string
	Number       int64
	Title        string
	Description  string
}

func (ref ExternalRef) valid() error {
	if ref.Source == "" || ref.Source == "internal" || ref.ExternalID == "" {
		return ErrInvalid.Msgf("external source and id required")
	}
	return nil
}

// owner is the live task that holds an external key, if any.
func (s *Store) owner(ctx context.Context, tx *oplog.Tx, source, externalID string) (Task, bool, error) {
	rows, err := s.Tasks().list(ctx, tx, Filter{Fields: map[string]any{"source": source, "external_id": externalID}, Limit: 1}, false)
	if err != nil || len(rows) == 0 {
		return Task{}, false, err
	}
	return rows[0], true, nil
}

// AdoptTask returns the task that holds ref's key. Without one it creates the
// task with AdoptedTaskID, or restores it when it was archived.
func (s *Store) AdoptTask(ctx context.Context, ref ExternalRef) (Task, error) {
	if err := ref.valid(); err != nil {
		return Task{}, err
	}
	var out Task
	spec, _ := schema.Lookup("task")
	err := s.log.Tx(ctx, func(tx *oplog.Tx) error {
		t, ok, err := s.owner(ctx, tx, ref.Source, ref.ExternalID)
		if err != nil || ok {
			out = t
			return err
		}
		id := AdoptedTaskID(ref.Source, ref.ExternalID)
		if _, err := s.Tasks().get(ctx, tx, id, true); err == nil {
			out, err = s.Tasks().patchTx(ctx, tx, spec, id, nil, Fields{"archived_at": nil})
			return err
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		var repo, number any
		if ref.RepositoryID != "" {
			repo = ref.RepositoryID
		}
		if ref.Number > 0 {
			number = json.Number(strconv.FormatInt(ref.Number, 10))
		}
		out, err = s.Tasks().write(ctx, tx, id, Fields{
			"source": ref.Source, "external_id": ref.ExternalID, "project_id": ref.ProjectID,
			"repository_id": repo, "number": number, "title": ref.Title, "description": ref.Description,
			"stage": "", "branch": "", "position": "", "created_at": now, "updated_at": now,
		})
		return err
	})
	return writeResult(out, err)
}

// AttachExternal gives an existing task the key of an external item, for
// instance the issue Dory created for it; the task keeps its id. It fails when
// another task already holds the key.
func (s *Store) AttachExternal(ctx context.Context, taskID string, ref ExternalRef) (Task, error) {
	if err := ref.valid(); err != nil {
		return Task{}, err
	}
	var out Task
	spec, _ := schema.Lookup("task")
	err := s.log.Tx(ctx, func(tx *oplog.Tx) error {
		t, ok, err := s.owner(ctx, tx, ref.Source, ref.ExternalID)
		if err != nil {
			return err
		}
		if ok {
			if t.ID != taskID {
				return ErrConflict.Msgf("%s item %s already belongs to task %s", ref.Source, ref.ExternalID, t.ID)
			}
			out = t
			return nil
		}
		fields := Fields{"source": ref.Source, "external_id": ref.ExternalID}
		if ref.RepositoryID != "" {
			fields["repository_id"] = ref.RepositoryID
		}
		if ref.Number > 0 {
			fields["number"] = json.Number(strconv.FormatInt(ref.Number, 10))
		}
		out, err = s.Tasks().patchTx(ctx, tx, spec, taskID, nil, fields)
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

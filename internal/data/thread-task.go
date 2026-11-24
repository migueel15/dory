package data

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
)

// EnsureThreadTask reuses the thread's task or creates and links one atomically.
func (s *Store) EnsureThreadTask(ctx context.Context, thread, title, description string) (Task, error) {
	var out Task
	err := s.log.Tx(ctx, func(tx *oplog.Tx) error {
		threads, tasks := s.Threads(), s.Tasks()
		th, err := threads.get(ctx, tx, thread, false)
		if err != nil {
			return err
		}
		if th.TaskID != nil && *th.TaskID != "" {
			out, err = tasks.get(ctx, tx, *th.TaskID, false)
			return err
		}
		if strings.TrimSpace(title) == "" {
			return fmt.Errorf("%w: title is required", ErrInvalid)
		}
		id := threadTaskID(thread)
		out, err = tasks.get(ctx, tx, id, true)
		if errors.Is(err, ErrNotFound) {
			spec, _ := schema.Lookup("task")
			fields, err := jsonFields(Fields{"project_id": th.ProjectID, "title": title, "description": description})
			if err != nil {
				return err
			}
			out, err = tasks.insert(ctx, tx, spec, id, fields)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if out.ArchivedAt != nil {
			return fmt.Errorf("%w: the task previously linked to this thread is archived", ErrInvalid)
		}
		spec, _ := schema.Lookup("thread")
		_, err = threads.patchTx(ctx, tx, spec, thread, nil, Fields{
			"task_id": id, "task_linked_at": s.now().UTC().Format(time.RFC3339Nano),
		})
		return err
	})
	return writeResult(out, err)
}

func threadTaskID(thread string) string {
	sum := sha256.Sum256([]byte("dory:plan-task:" + thread))
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

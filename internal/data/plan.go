package data

import (
	"context"
	"time"

	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
)

// AcceptPlan marks the supplied plan version accepted and moves its task
// from inbox to ready in the same transaction.
func (s *Store) AcceptPlan(ctx context.Context, p Plan) (Task, Plan, error) {
	var task Task
	var plan Plan
	err := s.log.Tx(ctx, func(tx *oplog.Tx) error {
		planSpec, _ := schema.Lookup("plan")
		at := s.now().UTC().Format(time.RFC3339Nano)
		var err error
		plan, err = s.Plans().patchTx(ctx, tx, planSpec, p.ID, &p.UpdatedAt, Fields{"accepted_at": at})
		if err != nil {
			return err
		}
		tasks := s.Tasks()
		task, err = tasks.get(ctx, tx, plan.TaskID, false)
		if err != nil || task.Stage != "inbox" {
			return err
		}
		taskSpec, _ := schema.Lookup("task")
		task, err = tasks.patchTx(ctx, tx, taskSpec, task.ID, nil, Fields{"stage": "ready"})
		return err
	})
	if err != nil {
		return Task{}, Plan{}, err
	}
	return task, plan, nil
}

package teamwork_runtime_change

import (
	"context"
	"fmt"
	"time"

	"media_agent/media_agent_gateway/biz/dal"
	taskdomain "media_agent/media_agent_gateway/biz/domain/agent_task"
	domain "media_agent/media_agent_gateway/biz/domain/teamwork_runtime"
)

type Result struct {
	Applied            bool
	CancelledTaskCount int
}

type Workflow struct {
	versions domain.Repository
	tasks    taskdomain.TopologyCancellationRepository
	outbox   taskdomain.OutboxRepository
	tx       dal.TxRunner
	now      func() time.Time
}

func New(versions domain.Repository, tasks taskdomain.TopologyCancellationRepository, outbox taskdomain.OutboxRepository, tx dal.TxRunner) *Workflow {
	return &Workflow{versions: versions, tasks: tasks, outbox: outbox, tx: tx, now: func() time.Time { return time.Now().UTC() }}
}

func (w *Workflow) Apply(ctx context.Context, eventID, workspaceID, topologyID string, version int64, status string) (Result, error) {
	event, err := domain.ValidateEvent(eventID, workspaceID, topologyID, version, status)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	err = w.tx.RunInTx(ctx, func(tx context.Context) error {
		applied, err := w.versions.ApplyEvent(tx, event, w.now())
		if err != nil {
			return err
		}
		if applied.Duplicate || !applied.Advanced {
			return nil
		}
		result.Applied = true
		tasks, err := w.tasks.CancelStaleTopology(tx, event.TopologyID, event.TopologyVersion, w.now())
		if err != nil {
			return err
		}
		for i := range tasks {
			payload := fmt.Sprintf(`{"task_id":%q,"status":"cancelled","reason":"topology_version_superseded"}`, tasks[i].ID)
			if err := w.outbox.Enqueue(tx, &taskdomain.OutboxEvent{AggregateID: tasks[i].ID, EventType: "agent_task.cancelled", PayloadJSON: payload, AvailableAt: w.now(), CreatedAt: w.now(), UpdatedAt: w.now()}); err != nil {
				return err
			}
		}
		result.CancelledTaskCount = len(tasks)
		return nil
	})
	return result, err
}

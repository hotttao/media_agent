package batch_run_dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"media_agent/media_agent_gateway/biz/dal"
	taskdomain "media_agent/media_agent_gateway/biz/domain/agent_task"
	domain "media_agent/media_agent_gateway/biz/domain/batch_run"
	taskservice "media_agent/media_agent_gateway/biz/service/agent_task"
)

type Dispatcher struct {
	batches domain.DispatchRepository
	tx      dal.TxRunner
	tasks   TaskApplication
	now     func() time.Time
}

type TaskApplication interface {
	Create(context.Context, taskservice.CreateCommand) (*taskdomain.Task, error)
	Cancel(context.Context, string, string) (*taskdomain.Task, error)
}

func NewDispatcher(batches domain.DispatchRepository, tx dal.TxRunner, tasks TaskApplication) *Dispatcher {
	return &Dispatcher{batches: batches, tx: tx, tasks: tasks, now: func() time.Time { return time.Now().UTC() }}
}

func (d *Dispatcher) Process(ctx context.Context, action, batchRunID string) error {
	switch action {
	case "batch_run.dispatch":
		return d.dispatch(ctx, batchRunID)
	case "batch_run.cancel":
		return d.cancel(ctx, batchRunID)
	default:
		return fmt.Errorf("unsupported Batch Run action %q", action)
	}
}

func (d *Dispatcher) dispatch(ctx context.Context, batchRunID string) error {
	batch, err := d.batches.Get(ctx, batchRunID)
	if err != nil {
		return err
	}
	if batch.Status == domain.StatusCancelled || batch.Status == domain.StatusCompleted || batch.Status == domain.StatusPartialFailed || batch.Status == domain.StatusFailed {
		return nil
	}
	for batch.NextCursor < batch.ItemCount {
		cursor := batch.NextCursor
		item := batch.Items[cursor]
		task, err := d.tasks.Create(ctx, taskservice.CreateCommand{Owner: batch.RequestedBy, WorkspaceID: batch.WorkspaceID, SessionID: item.SessionID, Source: "batch_run", SourceResourceID: batch.ID, IdempotencyKey: item.ItemKey, InputJSON: item.InputJSON})
		if err != nil {
			return err
		}
		err = d.tx.RunInTx(ctx, func(tx context.Context) error {
			return d.batches.AttachTask(tx, batch.ID, cursor, item.ItemKey, task.ID, d.now())
		})
		if errors.Is(err, domain.ErrConflict) {
			current, getErr := d.batches.Get(ctx, batch.ID)
			if getErr != nil {
				return getErr
			}
			if current.Status == domain.StatusCancelled {
				_, cancelErr := d.tasks.Cancel(ctx, task.ID, batch.RequestedBy)
				if cancelErr != nil && !errors.Is(cancelErr, taskdomain.ErrConflict) {
					return cancelErr
				}
				return nil
			}
			return err
		}
		if err != nil {
			return err
		}
		batch.NextCursor++
		batch.Status = domain.StatusRunning
	}
	return nil
}

func (d *Dispatcher) cancel(ctx context.Context, batchRunID string) error {
	batch, err := d.batches.Get(ctx, batchRunID)
	if err != nil {
		return err
	}
	if batch.Status != domain.StatusCancelled {
		return nil
	}
	var combined error
	for _, item := range batch.Items {
		if item.AgentTaskID == "" || item.Status == domain.ItemCompleted || item.Status == domain.ItemFailed || item.Status == domain.ItemCancelled {
			continue
		}
		_, cancelErr := d.tasks.Cancel(ctx, item.AgentTaskID, batch.RequestedBy)
		if cancelErr != nil && !errors.Is(cancelErr, taskdomain.ErrConflict) {
			combined = errors.Join(combined, cancelErr)
		}
	}
	return combined
}

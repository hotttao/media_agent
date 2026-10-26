package execute

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"media_agent/media_worker/biz/domain/execution"
	policy "media_agent/media_worker/biz/policy/execution"
	"media_agent/media_worker/biz/shared/correlation"
)

type Command struct {
	WorkerID     string
	Capabilities []execution.Capability
}
type Workflow struct {
	Gateway   execution.Gateway
	Executor  execution.Executor
	Results   execution.ResultRepository
	Recovery  execution.ExecutionRecovery
	Admission execution.Admission
}

func (w Workflow) Run(ctx context.Context, cmd Command) error {
	if err := w.Gateway.Register(ctx, cmd.WorkerID, cmd.Capabilities); err != nil {
		return fmt.Errorf("register worker: %w", err)
	}
	pending, err := w.Results.Pending()
	if err != nil {
		return err
	}
	for _, record := range pending {
		if _, err := DeliverStoredResult(ctx, w.Gateway, cmd.WorkerID, w.Results, record); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
	}
	if w.Recovery != nil {
		tasks, err := w.Recovery.PendingTasks(ctx)
		if err != nil {
			return err
		}
		for _, task := range tasks {
			release, err := acquireRecovery(ctx, w.Admission)
			if err != nil {
				return err
			}
			err = runTask(ctx, w.Gateway, cmd.WorkerID, task, w.Executor, w.Results, w.Recovery)
			release()
			if err != nil {
				return err
			}
			if ctx.Err() != nil {
				return nil
			}
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var once sync.Once
	var runErr error
	for _, capability := range cmd.Capabilities {
		for range capability.Concurrency {
			wg.Add(1)
			go func(cap execution.Capability) {
				defer wg.Done()
				if err := runCapability(ctx, w.Gateway, cmd.WorkerID, cap, w.Executor, w.Results, w.Recovery, w.Admission); err != nil {
					once.Do(func() { runErr = err; cancel() })
				}
			}(capability)
		}
	}
	wg.Wait()
	return runErr
}
func runCapability(ctx context.Context, client execution.Gateway, workerID string, capability execution.Capability, executor execution.Executor, store execution.ResultRepository, recovery execution.ExecutionRecovery, admission execution.Admission) error {
	for ctx.Err() == nil {
		release, err := acquire(ctx, admission)
		if err != nil {
			return err
		}
		task, err := client.Poll(ctx, workerID, capability)
		if err != nil {
			release()
			log.Printf("poll deferred capability=%s@%d", capability.Key, capability.Version)
			wait(ctx, time.Second)
			continue
		}
		if task == nil {
			release()
			wait(ctx, 300*time.Millisecond)
			continue
		}
		err = runTask(ctx, client, workerID, *task, executor, store, recovery)
		release()
		if err != nil {
			return err
		}
	}
	return nil
}

func DeliverStoredResult(ctx context.Context, client execution.Gateway, workerID string, store execution.ResultRepository, record execution.Result) (bool, error) {
	for ctx.Err() == nil {
		var err error
		if record.Reason != "" {
			_, err = client.FailWithKind(ctx, workerID, record.Task, record.Key, record.Reason, record.FailureKind)
		} else {
			_, err = client.Complete(ctx, workerID, record.Task, record.Key, record.Output)
		}
		if err == nil {
			if err := store.Finish(record, false); err != nil {
				return false, err
			}
			return true, nil
		}
		if policy.ResultRejected(err) {
			if err := store.Finish(record, true); err != nil {
				return false, err
			}
			log.Printf("event=worker_result_rejected trace_id=%s dispatch=%s", correlation.TraceID(record.Task.TraceParent), record.Task.DispatchID)
			return false, nil
		}
		log.Printf("event=worker_result_unconfirmed trace_id=%s dispatch=%s", correlation.TraceID(record.Task.TraceParent), record.Task.DispatchID)
		wait(ctx, time.Second)
	}
	return false, nil
}

func heartbeat(ctx context.Context, client execution.Gateway, workerID string, task execution.Task) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Heartbeat(ctx, workerID, task); err != nil && ctx.Err() == nil {
				log.Printf("event=worker_heartbeat_deferred trace_id=%s dispatch=%s", correlation.TraceID(task.TraceParent), task.DispatchID)
			}
		}
	}
}

func wait(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func acquire(ctx context.Context, admission execution.Admission) (func(), error) {
	if admission == nil {
		return func() {}, nil
	}
	return admission.Acquire(ctx)
}
func runTask(ctx context.Context, client execution.Gateway, workerID string, task execution.Task, executor execution.Executor, store execution.ResultRepository, recovery execution.ExecutionRecovery) error {
	taskCtx, cancelTask := context.WithCancel(correlation.WithParent(ctx, task.TraceParent))
	heartbeatDone := make(chan struct{})
	go func() { defer close(heartbeatDone); heartbeat(taskCtx, client, workerID, task) }()
	stopTask := func() { cancelTask(); <-heartbeatDone }
	var output map[string]any
	var executeErr error
	for ctx.Err() == nil {
		output, executeErr = executor.ExecuteContext(taskCtx, task)
		if !errors.Is(executeErr, execution.ErrExecutionPending) {
			break
		}
		wait(ctx, 5*time.Second)
	}
	if ctx.Err() != nil {
		stopTask()
		return nil
	}
	if errors.Is(executeErr, execution.ErrExecutionAbandoned) {
		stopTask()
		return nil
	}
	idempotencyKey := task.DispatchID + ":" + fmt.Sprint(task.Attempt)
	record := execution.Result{Task: task, Key: idempotencyKey + ":complete", Output: output}
	if executeErr != nil {
		record.Key, record.Output, record.Reason, record.FailureKind = idempotencyKey+":fail", nil, executeErr.Error(), policy.FailureKind(executeErr)
	}
	if err := store.Save(record); err != nil {
		stopTask()
		return err
	}
	submitted, err := DeliverStoredResult(ctx, client, workerID, store, record)
	if err != nil {
		stopTask()
		return err
	}
	stopTask()
	if recovery != nil && ctx.Err() == nil {
		if err := recovery.FinishTask(ctx, task); err != nil {
			return err
		}
	}
	if !submitted {
		return nil
	}
	if executeErr != nil {
		log.Printf("event=worker_task_failed trace_id=%s dispatch=%s capability=%s@%d", correlation.TraceID(task.TraceParent), task.DispatchID, task.CapabilityKey, task.CapabilityVersion)
		return nil
	}
	log.Printf("event=worker_task_completed trace_id=%s dispatch=%s capability=%s@%d", correlation.TraceID(task.TraceParent), task.DispatchID, task.CapabilityKey, task.CapabilityVersion)
	return nil
}

func acquireRecovery(ctx context.Context, a execution.Admission) (func(), error) {
	if recovery, ok := a.(execution.RecoveryAdmission); ok {
		return recovery.AcquireRecovery(ctx)
	}
	return acquire(ctx, a)
}

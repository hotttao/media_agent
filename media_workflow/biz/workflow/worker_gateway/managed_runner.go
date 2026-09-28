package worker_gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	jobdomain "media_agent/media_workflow/biz/domain/job"
)

type ManagedExecutor interface {
	Execute(context.Context, []byte, string) (map[string]any, error)
}

// Run gives each configured capability one serial execution loop. The flow's
// durable submitter delivers receipts independently of executor availability.
func (r *ManagedRuntime) Run(ctx context.Context, interval time.Duration, executors map[string]ManagedExecutor) error {
	if interval <= 0 {
		interval = time.Second
	}
	for key := range r.bindings {
		if executors[key] == nil {
			return fmt.Errorf("managed executor missing for %s", key)
		}
	}
	var workers sync.WaitGroup
	for key, binding := range r.bindings {
		workers.Add(1)
		go func(key string, binding ManagedBinding) {
			defer workers.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				if ctx.Err() != nil {
					return
				}
				dispatch, err := r.Poll(ctx, binding.CapabilityKey, binding.CapabilityVersion)
				if err != nil {
					log.Printf("managed poll failed: capability=%s", key)
				}
				if err == nil && dispatch != nil {
					if err := r.execute(ctx, dispatch, executors[key]); err != nil && ctx.Err() == nil {
						log.Printf("managed execution interrupted: dispatch_id=%s", dispatch.ID)
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}(key, binding)
	}
	workers.Wait()
	return ctx.Err()
}

func (r *ManagedRuntime) execute(ctx context.Context, dispatch *jobdomain.WorkerDispatch, executor ManagedExecutor) error {
	body, err := json.Marshal(map[string]any{
		"dispatch_id": dispatch.ID, "attempt": dispatch.Attempt, "requested_by": dispatch.UserID,
		"capability_key": dispatch.CapabilityKey, "capability_version": dispatch.CapabilityVersion,
		"input": json.RawMessage(dispatch.InputJSON),
	})
	if err != nil {
		return err
	}
	idempotencyKey := fmt.Sprintf("managed:%s:%d", dispatch.ID, dispatch.Attempt)
	if err := r.Heartbeat(ctx, dispatch.ID, dispatch.Attempt); err != nil {
		return err
	}
	executionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	heartbeatErr := make(chan error, 1)
	go func() {
		defer close(done)
		ticker := time.NewTicker(r.LeaseInterval())
		defer ticker.Stop()
		for {
			select {
			case <-executionCtx.Done():
				return
			case <-ticker.C:
				if err := r.Heartbeat(executionCtx, dispatch.ID, dispatch.Attempt); err != nil {
					heartbeatErr <- err
					cancel()
					return
				}
			}
		}
	}()
	output, err := executor.Execute(executionCtx, body, idempotencyKey)
	cancel()
	<-done
	if ctx.Err() != nil {
		return ctx.Err()
	}
	select {
	case err := <-heartbeatErr:
		return err
	default:
	}
	if err != nil {
		_, err = r.Fail(ctx, dispatch.ID, dispatch.Attempt, idempotencyKey, "managed executor failed")
		return err
	}
	_, err = r.Complete(ctx, dispatch.ID, dispatch.Attempt, idempotencyKey, output)
	if err == ErrInvalidOutput {
		_, err = r.Fail(ctx, dispatch.ID, dispatch.Attempt, idempotencyKey, "managed executor returned invalid candidates")
	}
	return err
}

package job

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

type nativeCancelStore struct {
	domain.ExecutionStartRepo
	job   *domain.Job
	calls int
	err   error
}

func (r *nativeCancelStore) CancelInitialStart(context.Context, string) (bool, error) {
	r.calls++
	if r.err != nil {
		return false, r.err
	}
	r.job.Status = domain.JobCancelled
	return true, nil
}
func (*nativeCancelStore) FinishCancelledStart(context.Context, domain.ExecutionStart, string) error {
	panic("delivery only")
}

func TestNativeCancelUsesEngineStateAndOnlyPersistsIntent(t *testing.T) {
	for _, status := range []domain.JobStatus{domain.JobRunning, domain.JobFailed, domain.JobSucceeded} {
		for _, native := range []string{"RUNNING", "PAUSED"} {
			t.Run(string(status)+"/"+native, func(t *testing.T) {
				svc, jobs, _ := newTestService(false)
				j := &domain.Job{ID: "job1", Origin: "native", Status: status, ConductorWorkflowID: "execution"}
				jobs.jobs[j.ID] = j
				store := &nativeCancelStore{job: j}
				engine := &mockWorkflowEngine{execution: &domain.EngineExecution{WorkflowInstanceID: "execution", Status: native}}
				svc.starts, svc.engine = store, engine
				got, err := svc.Cancel(context.Background(), j.ID)
				if err != nil || got.Status != domain.JobCancelled || store.calls != 1 || engine.terminated {
					t.Fatal("native cancel did not persist intent independently", err)
				}
				engine.execution = nil
				if _, err := svc.Cancel(context.Background(), j.ID); err != nil || store.calls != 1 {
					t.Fatal("cancel replay consulted engine or created another intent", err)
				}
			})
		}
	}
}

func TestNativeCancelRejectsTerminalOrUnknownExecution(t *testing.T) {
	for _, native := range []string{"COMPLETED", "FAILED", "TIMED_OUT", "TERMINATED", "wrong_instance", "missing"} {
		t.Run(native, func(t *testing.T) {
			svc, jobs, _ := newTestService(false)
			j := &domain.Job{ID: "job1", Origin: "native", Status: domain.JobRunning, ConductorWorkflowID: "execution"}
			jobs.jobs[j.ID] = j
			store := &nativeCancelStore{job: j}
			engine := &mockWorkflowEngine{execution: &domain.EngineExecution{WorkflowInstanceID: "execution", Status: native}}
			if native == "wrong_instance" {
				engine.execution.WorkflowInstanceID, engine.execution.Status = "other", "RUNNING"
			}
			if native == "missing" {
				engine.execution = nil
			}
			svc.starts, svc.engine = store, engine
			if _, err := svc.Cancel(context.Background(), j.ID); err == nil || store.calls != 0 || engine.terminated || j.Status != domain.JobRunning {
				t.Fatal("invalid execution was cancelled", err)
			}
		})
	}
}

func TestNativeCancelDoesNotTerminateWithoutDurableIntent(t *testing.T) {
	svc, jobs, _ := newTestService(false)
	j := &domain.Job{ID: "job1", Origin: "native", Status: domain.JobRunning, ConductorWorkflowID: "execution"}
	jobs.jobs[j.ID] = j
	store := &nativeCancelStore{job: j, err: errors.New("private database details")}
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "RUNNING"}}
	svc.starts, svc.engine = store, engine
	if _, err := svc.Cancel(context.Background(), j.ID); err == nil || err.Error() != "workflow cancellation was not persisted" || engine.terminated || j.Status != domain.JobRunning {
		t.Fatal("unpersisted cancellation reached engine", err)
	}
	svc.starts = nil
	if _, err := svc.Cancel(context.Background(), j.ID); err == nil || engine.terminated {
		t.Fatal("missing command silently used legacy termination", err)
	}
}

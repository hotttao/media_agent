package worker_gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	jobdomain "media_agent/media_workflow/biz/domain/job"
)

type nativeResultProbe struct {
	*taskEngineStub
	snapshot       *retryExecutionProbe
	lost, ignored  bool
	cancelOnUpdate bool
}

func (p *nativeResultProbe) Get(ctx context.Context, id string) (*jobdomain.EngineExecution, error) {
	return p.snapshot.Get(ctx, id)
}
func (p *nativeResultProbe) UpdateCapabilityTask(ctx context.Context, result jobdomain.CapabilityTaskResult) error {
	p.taskEngineStub.UpdateCapabilityTask(ctx, result)
	if p.cancelOnUpdate {
		p.snapshot.value.Status = "TERMINATED"
	}
	if !p.ignored {
		task := &p.snapshot.value.Tasks[0]
		task.Status, task.Output, task.ReasonForIncompletion = result.Status, result.Output, result.Reason
	}
	if p.lost {
		return errors.New("private transport error")
	}
	return nil
}

func TestHeartbeatCannotRenewTaskCancelledDuringNativeUpdate(t *testing.T) {
	f, repo, e, snapshot, poll := retryProjectionFixture()
	dispatch, err := f.Poll(context.Background(), poll)
	if err != nil {
		t.Fatal(err)
	}
	before := repo.dispatch.LeaseExpiresAt
	probe := &nativeResultProbe{taskEngineStub: e, snapshot: snapshot, cancelOnUpdate: true}
	f.Engine, f.Executions = probe, probe
	if _, err := f.Heartbeat(context.Background(), poll.Principal, dispatch.ID, poll.WorkerInstanceID, dispatch.Attempt); !errors.Is(err, jobdomain.ErrDispatchExpired) || !repo.dispatch.LeaseExpiresAt.Equal(before) || len(e.updateCalls) != 1 {
		t.Fatal("cancelled native task renewed local delivery bookkeeping", err)
	}
}

func TestDirectResultResolvesLostResponseWithoutInboxOrSecondUpdate(t *testing.T) {
	f, repo, e, snapshot, poll := retryProjectionFixture()
	probe := &nativeResultProbe{taskEngineStub: e, snapshot: snapshot, lost: true}
	f.Engine, f.Executions = probe, probe
	dispatch, err := f.Poll(context.Background(), poll)
	if err != nil {
		t.Fatal(err)
	}
	cmd := CompleteCommand{Principal: poll.Principal, DispatchID: dispatch.ID, WorkerInstanceID: poll.WorkerInstanceID, Attempt: dispatch.Attempt, IdempotencyKey: "stable", Output: map[string]any{"candidates": []any{map[string]any{"content": "result"}}}}
	first, err := f.Complete(context.Background(), cmd)
	if err != nil || first.ConductorStatus != "submitted" {
		t.Fatal("committed response loss was not observed", err)
	}
	repo.dispatch.LeaseExpiresAt = f.now().Add(-time.Minute)
	snapshot.value.Status = "COMPLETED"
	second, err := f.Complete(context.Background(), cmd)
	if err != nil || first.ID != second.ID || len(e.updateCalls) != 1 || repo.saveCalls != 0 {
		t.Fatal("confirmed result replay resent or wrote inbox", err)
	}
	cmd.Output = map[string]any{"candidates": []any{map[string]any{"content": "different"}}}
	if _, err := f.Complete(context.Background(), cmd); !errors.Is(err, jobdomain.ErrDispatchConflict) || len(e.updateCalls) != 1 {
		t.Fatal("conflicting replay overwrote native output")
	}
	if f.Jobs.(*jobRepoStub).item.Status != jobdomain.JobFailed {
		t.Fatal("proxy controlled execution state")
	}
}

func TestDirectResultDoesNotAcknowledgeIgnoredOrUncommittedUpdate(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   error
		calls  int
	}{
		{"IN_PROGRESS", ErrResultUnconfirmed, 1},
		{"TIMED_OUT", jobdomain.ErrDispatchConflict, 0},
		{"COMPLETED", jobdomain.ErrDispatchConflict, 0},
	} {
		t.Run(tc.status, func(t *testing.T) {
			f, repo, e, snapshot, poll := retryProjectionFixture()
			dispatch, err := f.Poll(context.Background(), poll)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.value.Tasks[0].Status = tc.status
			probe := &nativeResultProbe{taskEngineStub: e, snapshot: snapshot, ignored: true}
			f.Engine, f.Executions = probe, probe
			_, err = f.Complete(context.Background(), CompleteCommand{Principal: poll.Principal, DispatchID: dispatch.ID, WorkerInstanceID: poll.WorkerInstanceID, Attempt: dispatch.Attempt, IdempotencyKey: "stable", Output: map[string]any{"candidates": []any{map[string]any{"content": "result"}}}})
			if !errors.Is(err, tc.want) || len(e.updateCalls) != tc.calls || repo.markCalls != 0 || repo.saveCalls != 0 {
				t.Fatal("unconfirmed or obsolete result falsely acknowledged", err)
			}
		})
	}
}

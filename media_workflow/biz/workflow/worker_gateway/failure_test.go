package worker_gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/taskerror"
)

func TestFailureReceiptUsesNativeOutcomeAndPreservesLegacyReplay(t *testing.T) {
	for _, tc := range []struct{ kind, status string }{
		{"", "FAILED_WITH_TERMINAL_ERROR"},
		{FailurePermanent, "FAILED_WITH_TERMINAL_ERROR"},
		{FailureTransient, "FAILED"},
	} {
		t.Run("kind="+tc.kind, func(t *testing.T) {
			now := time.Now()
			flow, repo, engine := newPollFlow(now)
			repo.dispatch = &jobdomain.WorkerDispatch{
				ID: "dispatch-1", JobID: "job-1", ConductorWorkflowID: "workflow-1", ConductorTaskID: "task-1", Attempt: 2,
				UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1",
				AuthSource: "api_token", WorkerInstanceID: "desktop-1", Status: jobdomain.DispatchDelivered, LeaseExpiresAt: now.Add(time.Minute),
			}
			receipt, err := flow.Fail(context.Background(), FailCommand{
				Principal:  Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"},
				DispatchID: "dispatch-1", WorkerInstanceID: "desktop-1", Attempt: 2, IdempotencyKey: "failure-1", Reason: "provider unavailable", FailureKind: tc.kind,
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.kind != FailureTransient && string(receipt.ResultJSON) != `{"reason":"provider unavailable"}` {
				t.Fatal("legacy receipt digest changed")
			}
			if len(engine.updateCalls) != 1 {
				t.Fatal("expected one native result")
			}
			got := engine.updateCalls[0]
			if got.Status != tc.status || got.TaskID != "task-1" || got.Reason != "provider unavailable" || got.Output != nil {
				t.Fatalf("unexpected native result: %#v", got)
			}
		})
	}
}

type classifiedManagedExecutor struct{ err error }

func (e classifiedManagedExecutor) Execute(context.Context, []byte, string) (map[string]any, error) {
	return nil, e.err
}

func TestManagedExecutorFailureClassificationReachesNativeTask(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status string
	}{
		{taskerror.Retryable("provider_unavailable"), "FAILED"},
		{taskerror.Permanent("invalid_prompt"), "FAILED_WITH_TERMINAL_ERROR"},
	} {
		runtime, _, repo, engine := newManagedFixture(t)
		dispatch, err := runtime.Poll(context.Background(), "jimeng.image", 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := runtime.execute(context.Background(), dispatch, classifiedManagedExecutor{err: tc.err}); err != nil {
			t.Fatal(err)
		}
		if repo.saveCalls != 0 {
			t.Fatal("managed result wrote inbox")
		}
		if len(engine.updateCalls) != 2 || engine.updateCalls[1].Status != tc.status {
			t.Fatal("managed executor lost native failure classification")
		}
	}
}

func TestInvalidFailureClassificationDoesNotPersistOrReachEngine(t *testing.T) {
	flow, repo, engine := newPollFlow(time.Now())
	if _, err := flow.Fail(context.Background(), FailCommand{Reason: "bad", FailureKind: "retry_forever"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
	if repo.saveCalls != 0 || len(engine.updateCalls) != 0 {
		t.Fatal("invalid classification caused side effects")
	}
	err := func() error {
		_, err := flow.deliverResult(context.Background(), &jobdomain.WorkerDispatch{}, "fail", "key", map[string]any{"reason": "bad", "failure_kind": "retry_forever"})
		return err
	}()
	if !errors.Is(err, ErrInvalidRequest) || len(engine.updateCalls) != 0 {
		t.Fatal("invalid persisted classification reached engine")
	}
}

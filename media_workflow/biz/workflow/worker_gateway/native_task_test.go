package worker_gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	jobdomain "media_agent/media_workflow/biz/domain/job"
)

type retryExecutionProbe struct {
	value *jobdomain.EngineExecution
	err   error
	calls int
}

func (r *retryExecutionProbe) Get(context.Context, string) (*jobdomain.EngineExecution, error) {
	r.calls++
	return r.value, r.err
}

func retryProjectionFixture() (*Flow, *dispatchRepoStub, *taskEngineStub, *retryExecutionProbe, PollCommand) {
	f, d, e := newPollFlow(time.Now())
	c := f.Capabilities.(*capabilityRepoStub).item
	e.task.TaskType = c.ConductorTaskDef
	e.task.Input["capability_key"], e.task.Input["capability_version"] = c.Key, c.Version
	j := f.Jobs.(*jobRepoStub).item
	j.Origin, j.Status = "native", jobdomain.JobFailed
	r := &retryExecutionProbe{value: &jobdomain.EngineExecution{WorkflowInstanceID: j.ConductorWorkflowID, Status: "RUNNING", Tasks: []jobdomain.EngineTask{{
		TaskID: e.task.TaskID, TaskType: e.task.TaskType, Status: "IN_PROGRESS", RetryCount: e.task.RetryCount, Input: e.task.Input,
	}}}}
	f.Executions = r
	cmd := PollCommand{Principal: Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, WorkerInstanceID: "desktop-1", CapabilityKey: c.Key, CapabilityVersion: c.Version}
	return f, d, e, r, cmd
}

// Native activity, not a cached terminal state, determines dispatch eligibility.
func TestNativeRetryDeliversWithoutUnlockingCachedJob(t *testing.T) {
	for _, cached := range []jobdomain.JobStatus{jobdomain.JobFailed, jobdomain.JobSucceeded} {
		f, d, e, _, cmd := retryProjectionFixture()
		j := f.Jobs.(*jobRepoStub).item
		j.Status = cached
		got, err := f.Poll(context.Background(), cmd)
		if err != nil || got == nil || d.createCalls != 1 || len(e.updateCalls) != 0 || j.Status != cached {
			t.Fatal("native running task was blocked, postponed or changed the cache", err)
		}
		d.dispatch = got
		if _, err := f.Heartbeat(context.Background(), cmd.Principal, got.ID, cmd.WorkerInstanceID, got.Attempt); err != nil {
			t.Fatal("heartbeat rejected valid engine task", err)
		}
	}
}

func TestNativeRetryRejectsInvalidEngineTaskWithoutCallback(t *testing.T) {
	for _, scenario := range []string{"foreign_user", "foreign_org", "wrong_job_execution", "cancelled", "missing_reader", "wrong_engine_execution", "failed_engine", "missing_task", "duplicate_task", "terminal_task", "superseded_task", "wrong_attempt", "wrong_step", "wrong_native_job", "query_error"} {
		t.Run(scenario, func(t *testing.T) {
			f, d, e, r, cmd := retryProjectionFixture()
			j := f.Jobs.(*jobRepoStub).item
			switch scenario {
			case "foreign_user":
				cmd.UserID = "other"
			case "foreign_org":
				cmd.OrganizationID = "other"
			case "wrong_job_execution":
				j.ConductorWorkflowID = "other"
			case "cancelled":
				j.Status = jobdomain.JobCancelled
			case "missing_reader":
				f.Executions = nil
			case "wrong_engine_execution":
				r.value.WorkflowInstanceID = "other"
			case "failed_engine":
				r.value.Status = "FAILED"
			case "missing_task":
				r.value.Tasks = nil
			case "duplicate_task":
				r.value.Tasks = append(r.value.Tasks, r.value.Tasks[0])
			case "terminal_task":
				r.value.Tasks[0].Status = "COMPLETED"
			case "superseded_task":
				r.value.Tasks[0].Retried = true
			case "wrong_attempt":
				r.value.Tasks[0].RetryCount++
			case "wrong_step":
				f.StepRuns.(*stepRunRepoStub).item.JobID = "other"
			case "wrong_native_job":
				r.value.Tasks[0].Input = map[string]any{"job_id": "other"}
			case "query_error":
				r.err = errors.New("private upstream secret")
			}
			got, err := f.Poll(context.Background(), cmd)
			if err == nil || got != nil || d.createCalls != 0 || len(e.updateCalls) != 0 {
				t.Fatal("invalid native task was delivered or updated", err)
			}
			if err.Error() == "private upstream secret" {
				t.Fatal("unsafe upstream error exposed")
			}
		})
	}
}

func TestManagedNativeRetryDeliversWithoutCachedStateReset(t *testing.T) {
	f, d, e, _, _ := retryProjectionFixture()
	c := f.Capabilities.(*capabilityRepoStub).item
	c.ExecutionTarget, c.ManagedPoolKey, c.Status = "managed_worker", "gpu-cn", "published"
	runtime, err := NewManagedRuntime(context.Background(), f, "managed", "process-one", []ManagedBinding{{CapabilityKey: c.Key, CapabilityVersion: c.Version, PoolKey: c.ManagedPoolKey}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := runtime.Poll(context.Background(), c.Key, c.Version)
	if err != nil || got == nil || d.createCalls != 1 || len(e.updateCalls) != 0 || got.AuthSource != "internal_runtime" || got.ActorID != runtime.actorID {
		t.Fatal("managed native task blocked by cached failure", err)
	}
}

package worker_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	capabilitydomain "media_agent/media_workflow/biz/domain/capability"
	jobdomain "media_agent/media_workflow/biz/domain/job"
)

type capabilityRepoStub struct {
	capabilitydomain.Repo
	item *capabilitydomain.Capability
}

func (r *capabilityRepoStub) Get(context.Context, string, int) (*capabilitydomain.Capability, error) {
	return r.item, nil
}

type jobRepoStub struct {
	jobdomain.JobRepo
	item *jobdomain.Job
}

func (r *jobRepoStub) Get(context.Context, string) (*jobdomain.Job, error) { return r.item, nil }

type stepRunRepoStub struct {
	jobdomain.StepRunRepo
	item *jobdomain.StepRun
}

func (r *stepRunRepoStub) GetByJobAndStepKey(context.Context, string, string) (*jobdomain.StepRun, error) {
	return r.item, nil
}

type taskEngineStub struct {
	task        *jobdomain.CapabilityTask
	pollType    string
	pollWorker  string
	pollDomain  string
	updateCalls []jobdomain.CapabilityTaskResult
}

func (e *taskEngineStub) PollCapabilityTask(_ context.Context, taskType, workerID, executionDomain string) (*jobdomain.CapabilityTask, error) {
	e.pollType, e.pollWorker, e.pollDomain = taskType, workerID, executionDomain
	return e.task, nil
}

func (e *taskEngineStub) UpdateCapabilityTask(_ context.Context, result jobdomain.CapabilityTaskResult) error {
	e.updateCalls = append(e.updateCalls, result)
	return nil
}

type dispatchRepoStub struct {
	jobdomain.DispatchRepo
	active         *jobdomain.WorkerDispatch
	created        *jobdomain.WorkerDispatch
	dispatch       *jobdomain.WorkerDispatch
	receipt        *jobdomain.WorkerResultReceipt
	findActiveCall int
	createCalls    int
	saveCalls      int
	markCalls      int
}

func (r *dispatchRepoStub) FindActive(context.Context, string, string, string, string, string, int, time.Time) (*jobdomain.WorkerDispatch, error) {
	r.findActiveCall++
	if r.active == nil {
		return nil, jobdomain.ErrDispatchNotFound
	}
	return r.active, nil
}

func (r *dispatchRepoStub) Create(_ context.Context, dispatch *jobdomain.WorkerDispatch) (*jobdomain.WorkerDispatch, error) {
	r.createCalls++
	r.created = dispatch
	r.dispatch = dispatch
	return dispatch, nil
}

func (r *dispatchRepoStub) Get(context.Context, string) (*jobdomain.WorkerDispatch, error) {
	if r.dispatch == nil {
		return nil, jobdomain.ErrDispatchNotFound
	}
	return r.dispatch, nil
}

func (r *dispatchRepoStub) SaveResult(_ context.Context, _ string, _ int, operation, idempotencyKey string, resultJSON []byte, _ string, _ time.Time) (*jobdomain.WorkerResultReceipt, error) {
	r.saveCalls++
	if r.receipt == nil {
		r.receipt = &jobdomain.WorkerResultReceipt{
			ID: "receipt-1", DispatchID: r.dispatch.ID, Attempt: r.dispatch.Attempt,
			Operation: operation, IdempotencyKey: idempotencyKey, ResultJSON: resultJSON,
			ConductorStatus: "pending", Dispatch: r.dispatch,
		}
	}
	return r.receipt, nil
}

func (r *dispatchRepoStub) MarkResultSubmitted(context.Context, string, time.Time) error {
	r.markCalls++
	r.receipt.ConductorStatus = "submitted"
	return nil
}

func newPollFlow(now time.Time) (*Flow, *dispatchRepoStub, *taskEngineStub) {
	dispatches := &dispatchRepoStub{}
	engine := &taskEngineStub{task: &jobdomain.CapabilityTask{
		TaskID: "task-1", WorkflowInstanceID: "workflow-1", RetryCount: 1,
		Input: map[string]any{"job_id": "job-1", "step_key": "generate"},
	}}
	return &Flow{
		Dispatches: dispatches,
		Jobs: &jobRepoStub{item: &jobdomain.Job{
			ID: "job-1", SubmittedBy: "user-1", ConductorWorkflowID: "workflow-1", Status: jobdomain.JobRunning,
		}},
		StepRuns: &stepRunRepoStub{item: &jobdomain.StepRun{ID: "step-run-1", JobID: "job-1", StepKey: "generate"}},
		Capabilities: &capabilityRepoStub{item: &capabilitydomain.Capability{
			Key: "jimeng.image", Version: 1, ConductorTaskDef: "jimeng_image_v1", ExecutionTarget: capabilitydomain.ExecutionTargetPersonalWorker,
		}},
		Engine: engine, Now: func() time.Time { return now }, Lease: time.Minute,
	}, dispatches, engine
}

func TestRegisterRequiresPrincipalAndPublishedCapability(t *testing.T) {
	flow, _, _ := newPollFlow(time.Now())
	err := flow.Register(context.Background(), Principal{}, "desktop-1", []CapabilityVersion{{Key: "jimeng.image", Version: 1, Concurrency: 1}})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Register() error = %v", err)
	}
	err = flow.Register(context.Background(), Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, "desktop-1", []CapabilityVersion{{Key: "jimeng.image", Version: 1, Concurrency: 1}})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestPersonalWorkerGatewayRejectsManagedCapability(t *testing.T) {
	flow, _, engine := newPollFlow(time.Now())
	capability := flow.Capabilities.(*capabilityRepoStub).item
	capability.ExecutionTarget = capabilitydomain.ExecutionTargetManagedWorker
	capability.ManagedPoolKey = "gpu-cn"
	principal := Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}

	err := flow.Register(context.Background(), principal, "desktop-1", []CapabilityVersion{{Key: capability.Key, Version: capability.Version, Concurrency: 1}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Register() error = %v, want forbidden", err)
	}
	_, err = flow.Poll(context.Background(), PollCommand{
		Principal: principal, WorkerInstanceID: "desktop-1", CapabilityKey: capability.Key, CapabilityVersion: capability.Version,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Poll() error = %v, want forbidden", err)
	}
	if engine.pollType != "" {
		t.Fatalf("managed capability reached Conductor poll: %q", engine.pollType)
	}
}

func TestPollUsesOpaqueUserDomainAndPersistsBeforeDelivery(t *testing.T) {
	now := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	flow, dispatches, engine := newPollFlow(now)
	principal := Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}

	dispatch, err := flow.Poll(context.Background(), PollCommand{
		Principal: principal, WorkerInstanceID: "desktop-1", CapabilityKey: "jimeng.image", CapabilityVersion: 1,
	})
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if engine.pollType != "jimeng_image_v1" || engine.pollWorker != "media-workflow-proxy" {
		t.Fatalf("unexpected Conductor poll: type=%q worker=%q", engine.pollType, engine.pollWorker)
	}
	if engine.pollDomain != jobdomain.ExecutionDomainForUser("user-1") {
		t.Fatalf("poll domain = %q", engine.pollDomain)
	}
	if dispatches.createCalls != 1 || dispatch.ID == "" || dispatch.Attempt != 2 {
		t.Fatalf("dispatch was not durably created: %#v", dispatch)
	}
	var input map[string]any
	if err := json.Unmarshal(dispatch.InputJSON, &input); err != nil || input["job_id"] != "job-1" {
		t.Fatalf("dispatch input = %s, err = %v", dispatch.InputJSON, err)
	}
}

func TestPollReturnsActiveDispatchWithoutPollingConductor(t *testing.T) {
	flow, dispatches, engine := newPollFlow(time.Now())
	dispatches.active = &jobdomain.WorkerDispatch{ID: "dispatch-existing"}

	got, err := flow.Poll(context.Background(), PollCommand{
		Principal:        Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"},
		WorkerInstanceID: "desktop-1", CapabilityKey: "jimeng.image", CapabilityVersion: 1,
	})
	if err != nil || got.ID != "dispatch-existing" {
		t.Fatalf("Poll() = %#v, %v", got, err)
	}
	if engine.pollType != "" || dispatches.createCalls != 0 {
		t.Fatal("active dispatch should be returned without polling Conductor")
	}
}

func TestCompleteIsIdempotentAfterReceiptWasSubmitted(t *testing.T) {
	now := time.Now()
	flow, dispatches, engine := newPollFlow(now)
	dispatches.dispatch = &jobdomain.WorkerDispatch{
		ID: "dispatch-1", ConductorTaskID: "task-1", ConductorWorkflowID: "workflow-1", Attempt: 1,
		UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token", WorkerInstanceID: "desktop-1",
		Status: jobdomain.DispatchDelivered, LeaseExpiresAt: now.Add(time.Minute),
	}
	cmd := CompleteCommand{
		Principal:  Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"},
		DispatchID: "dispatch-1", WorkerInstanceID: "desktop-1", Attempt: 1, IdempotencyKey: "result-1",
		Output: map[string]any{"candidates": []any{map[string]any{"url": "https://example.test/result.png"}}},
	}
	if _, err := flow.Complete(context.Background(), cmd); err != nil {
		t.Fatalf("first Complete() error = %v", err)
	}
	if _, err := flow.Complete(context.Background(), cmd); err != nil {
		t.Fatalf("second Complete() error = %v", err)
	}
	if dispatches.saveCalls != 2 || len(engine.updateCalls) != 0 || dispatches.markCalls != 0 {
		t.Fatalf("save=%d updates=%d marks=%d", dispatches.saveCalls, len(engine.updateCalls), dispatches.markCalls)
	}
	if err := flow.SubmitReceipt(context.Background(), dispatches.receipt); err != nil {
		t.Fatalf("SubmitReceipt() error = %v", err)
	}
	if len(engine.updateCalls) != 1 || dispatches.markCalls != 1 {
		t.Fatalf("updates=%d marks=%d", len(engine.updateCalls), dispatches.markCalls)
	}
}

func TestHeartbeatRejectsForeignAndStaleDispatches(t *testing.T) {
	now := time.Now()
	flow, dispatches, _ := newPollFlow(now)
	dispatches.dispatch = &jobdomain.WorkerDispatch{
		ID: "dispatch-1", Attempt: 2, UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token",
		WorkerInstanceID: "desktop-1", Status: jobdomain.DispatchDelivered, LeaseExpiresAt: now.Add(time.Minute),
	}

	_, err := flow.Heartbeat(context.Background(), Principal{UserID: "other", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, "dispatch-1", "desktop-1", 2)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign Heartbeat() error = %v", err)
	}
	_, err = flow.Heartbeat(context.Background(), Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, "dispatch-1", "desktop-1", 1)
	if !errors.Is(err, jobdomain.ErrDispatchConflict) {
		t.Fatalf("stale Heartbeat() error = %v", err)
	}
	dispatches.dispatch.LeaseExpiresAt = now.Add(-time.Second)
	_, err = flow.Heartbeat(context.Background(), Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, "dispatch-1", "desktop-1", 2)
	if !errors.Is(err, jobdomain.ErrDispatchExpired) {
		t.Fatalf("expired Heartbeat() error = %v", err)
	}
}

func TestCompleteRejectsCancelledJob(t *testing.T) {
	now := time.Now()
	flow, dispatches, engine := newPollFlow(now)
	flow.Jobs.(*jobRepoStub).item.Status = jobdomain.JobCancelled
	dispatches.dispatch = &jobdomain.WorkerDispatch{
		ID: "dispatch-1", JobID: "job-1", Attempt: 1, UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token",
		WorkerInstanceID: "desktop-1", Status: jobdomain.DispatchDelivered, LeaseExpiresAt: now.Add(time.Minute),
	}
	_, err := flow.Complete(context.Background(), CompleteCommand{
		Principal:  Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"},
		DispatchID: "dispatch-1", WorkerInstanceID: "desktop-1", Attempt: 1, IdempotencyKey: "result-1",
		Output: map[string]any{"candidates": []any{map[string]any{"url": "https://example.test/result.png"}}},
	})
	if !errors.Is(err, jobdomain.ErrDispatchExpired) {
		t.Fatalf("Complete() error = %v", err)
	}
	if dispatches.saveCalls != 0 || len(engine.updateCalls) != 0 {
		t.Fatal("cancelled job result must not be persisted or forwarded")
	}
}

func TestCompleteRejectsEmptyCandidateList(t *testing.T) {
	flow, _, _ := newPollFlow(time.Now())
	_, err := flow.Complete(context.Background(), CompleteCommand{Output: map[string]any{"candidates": []any{}}})
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("Complete() error = %v", err)
	}
}

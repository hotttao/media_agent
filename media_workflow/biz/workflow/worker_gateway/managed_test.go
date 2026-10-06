package worker_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	capabilitydomain "media_agent/media_workflow/biz/domain/capability"
	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

type managedExecutorStub struct {
	fail   bool
	called bool
}

func (e *managedExecutorStub) Execute(_ context.Context, body []byte, key string) (map[string]any, error) {
	var input map[string]any
	if err := json.Unmarshal(body, &input); err != nil || input["requested_by"] != "user-1" || key == "" {
		return nil, errors.New("invalid envelope")
	}
	e.called = true
	if e.fail {
		return nil, errors.New("executor unavailable")
	}
	return map[string]any{"candidates": []any{map[string]any{"content": "result"}}}, nil
}

func TestManagedRunnerHeartbeatsAndStoresSuccessOrFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		runtime, _, repo, engine := newManagedFixture(t)
		dispatch, err := runtime.Poll(context.Background(), "jimeng.image", 1)
		if err != nil {
			t.Fatal(err)
		}
		executor := &managedExecutorStub{fail: fail}
		if err := runtime.execute(context.Background(), dispatch, executor); err != nil {
			t.Fatal(err)
		}
		if !executor.called || len(engine.updateCalls) != 1 || engine.updateCalls[0].Status != "IN_PROGRESS" {
			t.Fatal("execution was not leased before provider call")
		}
		want := "complete"
		if fail {
			want = "fail"
		}
		if repo.receipt == nil || repo.receipt.Operation != want || repo.receipt.ConductorStatus != "pending" {
			t.Fatal("result did not reach durable inbox")
		}
	}
}

func TestManagedRuntimeRejectsWrongFrozenCapabilityVersion(t *testing.T) {
	runtime, _, repo, engine := newManagedFixture(t)
	engine.task.Input["capability_version"] = 2
	if _, err := runtime.Poll(context.Background(), "jimeng.image", 1); !errors.Is(err, ErrInvalidTask) {
		t.Fatalf("wrong version accepted: %v", err)
	}
	if repo.createCalls != 0 {
		t.Fatal("wrong version persisted")
	}
}

func newManagedFixture(t *testing.T) (*ManagedRuntime, *Flow, *dispatchRepoStub, *taskEngineStub) {
	t.Helper()
	flow, dispatches, engine := newPollFlow(time.Date(2026, 9, 28, 1, 0, 0, 0, time.UTC))
	capability := flow.Capabilities.(*capabilityRepoStub).item
	capability.Status = "published"
	capability.ExecutionTarget = capabilitydomain.ExecutionTargetManagedWorker
	capability.ManagedPoolKey = "gpu-cn"
	engine.task.Input["capability_key"] = capability.Key
	engine.task.Input["capability_version"] = capability.Version
	runtime, err := NewManagedRuntime(context.Background(), flow, "server-worker", "instance-1", []ManagedBinding{{CapabilityKey: capability.Key, CapabilityVersion: 1, PoolKey: "gpu-cn"}})
	if err != nil {
		t.Fatal(err)
	}
	return runtime, flow, dispatches, engine
}

func TestManagedPollIsolatesPoolAndRecordsJobUser(t *testing.T) {
	runtime, _, _, engine := newManagedFixture(t)
	dispatch, err := runtime.Poll(context.Background(), "jimeng.image", 1)
	if err != nil {
		t.Fatal(err)
	}
	if engine.pollDomain != jobdomain.ExecutionDomainForManagedPool("gpu-cn") || engine.pollWorker != "instance-1" {
		t.Fatalf("invalid routing: %#v", engine)
	}
	if dispatch.UserID != "user-1" || dispatch.OrganizationID != "org-1" || dispatch.ActorID != "Worker:server-worker" || dispatch.CredentialID != "" || dispatch.AuthSource != "internal_runtime" {
		t.Fatalf("invalid managed identity: %#v", dispatch)
	}
}

func TestManagedRuntimeRejectsPersonalAndUnlistedCapabilities(t *testing.T) {
	runtime, flow, _, engine := newManagedFixture(t)
	if _, err := runtime.Poll(context.Background(), "other", 1); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unlisted capability: %v", err)
	}
	flow.Capabilities.(*capabilityRepoStub).item.ExecutionTarget = capabilitydomain.ExecutionTargetPersonalWorker
	if _, err := runtime.Poll(context.Background(), "jimeng.image", 1); !errors.Is(err, ErrForbidden) {
		t.Fatalf("personal capability: %v", err)
	}
	if engine.pollType != "" {
		t.Fatal("rejected request reached Conductor")
	}
}

func TestManagedRuntimeRejectsPoolMismatch(t *testing.T) {
	_, flow, _, _ := newManagedFixture(t)
	_, err := NewManagedRuntime(context.Background(), flow, "server-worker", "instance-1", []ManagedBinding{{CapabilityKey: "jimeng.image", CapabilityVersion: 1, PoolKey: "other-pool"}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("pool mismatch: %v", err)
	}
}

func TestManagedResultUsesDurableInboxAndCannotUsePublicComplete(t *testing.T) {
	runtime, flow, repo, engine := newManagedFixture(t)
	dispatch, err := runtime.Poll(context.Background(), "jimeng.image", 1)
	if err != nil {
		t.Fatal(err)
	}
	output := map[string]any{"candidates": []any{map[string]any{"url": "https://example.test/result.png"}}}
	receipt, err := runtime.Complete(context.Background(), dispatch.ID, dispatch.Attempt, "result-1", output)
	if err != nil || receipt == nil || repo.saveCalls != 1 || len(engine.updateCalls) != 0 {
		t.Fatalf("durable result: %v", err)
	}
	if err := flow.SubmitReceipt(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if len(engine.updateCalls) != 1 || engine.updateCalls[0].Status != "COMPLETED" {
		t.Fatal("result not delivered to Conductor")
	}
	_, err = flow.Complete(context.Background(), CompleteCommand{
		Principal:  Principal{UserID: dispatch.UserID, ActorID: dispatch.ActorID, AuthSource: "internal_runtime"},
		DispatchID: dispatch.ID, WorkerInstanceID: dispatch.WorkerInstanceID, Attempt: dispatch.Attempt, IdempotencyKey: "result-1", Output: output,
	})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("public API accepted internal principal: %v", err)
	}
}

func TestManagedCompletionRejectsOtherWorkerAndExpiredLease(t *testing.T) {
	runtime, _, repo, _ := newManagedFixture(t)
	dispatch, err := runtime.Poll(context.Background(), "jimeng.image", 1)
	if err != nil {
		t.Fatal(err)
	}
	output := map[string]any{"candidates": []any{map[string]any{"content": "result"}}}
	repo.dispatch.ActorID = "Worker:other"
	if _, err := runtime.Complete(context.Background(), dispatch.ID, dispatch.Attempt, "result-1", output); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other worker: %v", err)
	}
	repo.dispatch.ActorID = "Worker:server-worker"
	repo.dispatch.LeaseExpiresAt = runtime.flow.now().Add(-time.Second)
	if _, err := runtime.Complete(context.Background(), dispatch.ID, dispatch.Attempt, "result-1", output); !errors.Is(err, jobdomain.ErrDispatchExpired) {
		t.Fatalf("expired lease: %v", err)
	}
	if repo.saveCalls != 0 {
		t.Fatal("unauthorized result persisted")
	}
}

func TestManagedInputDeliveryUsesJobOrganizationAndStableSnapshot(t *testing.T) {
	runtime, flow, repo, engine := newManagedFixture(t)
	engine.task.Input["organization_id"] = "untrusted-task-org"
	engine.task.Input["images"] = []any{"object_path:materials/org-1/image.png"}
	flow.ImportedAssets = importedAssetStub{object: client.ResolvedStorageObject{URL: "https://storage.test/image.png?fresh=1", ExpiresAt: flow.now().Add(time.Hour).Unix()}}
	first, err := runtime.Poll(context.Background(), "jimeng.image", 1)
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	if json.Unmarshal(first.InputJSON, &input) != nil || input["images"].([]any)[0] != "https://storage.test/image.png?fresh=1" || repo.created.OrganizationID != "org-1" {
		t.Fatal("managed delivery used task tenant or failed to resolve media")
	}
	repo.active = repo.created
	engine.pollType = ""
	flow.ImportedAssets = importedAssetStub{err: errors.New("unavailable storage")}
	if got, err := runtime.Poll(context.Background(), "jimeng.image", 1); err == nil || got != nil || engine.pollType != "" || repo.createCalls != 1 {
		t.Fatal("unavailable media consumed another managed task")
	}
	flow.ImportedAssets = importedAssetStub{object: client.ResolvedStorageObject{URL: "https://storage.test/image.png?fresh=2", ExpiresAt: flow.now().Add(time.Hour).Unix()}}
	second, err := runtime.Poll(context.Background(), "jimeng.image", 1)
	if err != nil || second.ID != first.ID {
		t.Fatal("managed lease could not be redelivered", err)
	}
	if json.Unmarshal(second.InputJSON, &input) != nil || input["images"].([]any)[0] != "https://storage.test/image.png?fresh=2" {
		t.Fatal("managed redelivery reused a stale URL")
	}
	if json.Unmarshal(repo.active.InputJSON, &input) != nil || input["images"].([]any)[0] != "object_path:materials/org-1/image.png" {
		t.Fatal("managed snapshot was rewritten")
	}
}

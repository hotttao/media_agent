//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	httpclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/datatypes"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	capabilityrepo "media_agent/media_workflow/biz/dal/repo/capability"
	jobrepo "media_agent/media_workflow/biz/dal/repo/job"
	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/worker_gateway"
)

func newDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("workflow_e2e"), postgres.WithUsername("test"), postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Error(err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if err := dal.Migrate("../migrations", &configpb.DatabaseConfig{Dsn: dsn, MigrateOnStart: true}); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(pgdriver.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn, _ := db.DB(); _ = conn.Close() })
	return db
}

// Real Conductor + isolated PostgreSQL + a mock provider. No production Job,
// credential, or capability is changed. Provider is the only mocked boundary.
func TestManagedWorkerConductorRoundTrip(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL for real Conductor integration")
	}
	db := newDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	name := "managed_e2e_" + id
	pool := "e2e_" + id
	api := conductor.NewClient(address)
	if err := api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{{Name: name, RetryCount: 0, TimeoutSeconds: 120, ResponseTimeoutSeconds: 60, TimeoutPolicy: "TIME_OUT_WF"}}); err != nil {
		t.Fatal(err)
	}
	definition := compiler.Definition{Name: name, Version: 1, SchemaVersion: 2,
		Tasks: []compiler.Task{{Name: name, TaskReferenceName: "image", Type: "SIMPLE", InputParameters: map[string]any{
			"job_id": "${workflow.input.job_id}", "step_key": "image", "capability_key": name, "capability_version": 1, "prompt": "test image",
		}}}, OutputParameters: map[string]any{"candidates": "${image.output.candidates}"}}
	if err := api.RegisterWorkflow(ctx, definition); err != nil {
		t.Fatal(err)
	}
	// Delete only definitions created by this test; never touch user records.
	t.Cleanup(func() {
		for _, path := range []string{"/api/metadata/workflow/" + name + "/1", "/api/metadata/taskdefs/" + name} {
			req, _ := http.NewRequest(http.MethodDelete, address+path, nil)
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				t.Error(err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				t.Errorf("definition cleanup: %d", resp.StatusCode)
			}
		}
	})
	jobID := "job_" + id
	workflowID, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: name, Version: 1,
		Input: map[string]any{"job_id": jobID}, TaskToDomain: map[string]string{name: jobdomain.ExecutionDomainForManagedPool(pool)}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = api.TerminateWorkflow(context.Background(), workflowID, "isolated test cleanup") })
	job := &model.Job{ID: jobID, Scope: "e2e", WorkflowID: name, WorkflowVersion: 1,
		Input: datatypes.JSON(`{}`), Status: "running", RetryHistory: datatypes.JSON(`[]`), SubmittedBy: "e2e-owner", SubmittedAt: time.Now(), ConductorWorkflowID: workflowID}
	if err := db.Create(job).Error; err != nil {
		t.Fatal(err)
	}
	step := &model.StepRun{ID: "step_" + id, JobID: jobID, StepKey: "image", StepOrder: 1, TemplateID: "e2e", TemplateVersion: 1, Status: "pending"}
	if err := db.Create(step).Error; err != nil {
		t.Fatal(err)
	}
	capability := &model.ExecutionCapability{CapabilityKey: name, Version: 1, Provider: "mock", Mode: "image",
		InputSchema: []byte(`[]`), OutputAssetKind: "image", ConductorTaskDef: name, ExecutionTarget: "managed_worker", ManagedPoolKey: &pool,
		RetryPolicy: []byte(`{}`), TimeoutSeconds: 120, Status: "published", CreatedAt: time.Now()}
	if err := db.Create(capability).Error; err != nil {
		t.Fatal(err)
	}
	repo := jobrepo.NewDispatchRepo(db)
	flow := &worker_gateway.Flow{Dispatches: repo, Jobs: jobrepo.NewJobRepo(db), StepRuns: jobrepo.NewStepRunRepo(db),
		Capabilities: capabilityrepo.NewRepo(db), Engine: &conductor.ExecutionEngine{Client: api}}
	runtime, err := worker_gateway.NewManagedRuntime(ctx, flow, "server-e2e", "instance-e2e", []worker_gateway.ManagedBinding{{CapabilityKey: name, CapabilityVersion: 1, PoolKey: pool}})
	if err != nil {
		t.Fatal(err)
	}
	// Other pools cannot consume this task.
	otherCtx, otherCancel := context.WithTimeout(ctx, time.Second)
	otherTask, _ := api.Poll(otherCtx, name, "other-worker", jobdomain.ExecutionDomainForManagedPool("other-"+id))
	otherCancel()
	if otherTask != nil {
		t.Fatal("managed task leaked to another pool")
	}
	dispatch, err := runtime.Poll(ctx, name, 1)
	if err != nil || dispatch == nil {
		t.Fatalf("poll: %v", err)
	}
	if dispatch.UserID != "e2e-owner" || dispatch.ActorID != "Worker:server-e2e" || dispatch.CredentialID != "" || dispatch.AuthSource != "internal_runtime" {
		t.Fatal("identity audit mismatch")
	}
	resumed, err := runtime.Poll(ctx, name, 1)
	if err != nil || resumed.ID != dispatch.ID {
		t.Fatal("active dispatch not resumed")
	}
	collision := *dispatch
	collision.ID = "other-" + id
	collision.ActorID = "Worker:other"
	if _, err := repo.Create(ctx, &collision); !errors.Is(err, jobdomain.ErrDispatchConflict) {
		t.Fatalf("owner collision: %v", err)
	}
	if _, err := repo.FindActiveManaged(ctx, "Worker:other", "instance-e2e", name, 1, time.Now()); !errors.Is(err, jobdomain.ErrDispatchNotFound) {
		t.Fatalf("dispatch leaked: %v", err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope map[string]any
		if r.URL.Path != "/execute" || r.Header.Get("Authorization") != "" || r.Header.Get("Idempotency-Key") == "" || json.NewDecoder(r.Body).Decode(&envelope) != nil || envelope["requested_by"] != "e2e-owner" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"url":"https://example.test/mock-image.png"}]}`))
	}))
	defer provider.Close()
	httpClient, err := httpclient.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- runtime.Run(runCtx, 20*time.Millisecond, map[string]worker_gateway.ManagedExecutor{name + "@1": &client.ManagedExecutor{Client: httpClient, BaseURL: provider.URL}})
	}()
	defer func() { stop(); <-done }()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var receipt *jobdomain.WorkerResultReceipt
	for receipt == nil {
		pending, err := repo.ListPendingResults(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) == 1 {
			receipt = pending[0]
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("result inbox did not converge")
		case <-ticker.C:
		}
	}
	if receipt.Operation != "complete" || receipt.ConductorStatus != "pending" {
		t.Fatal("invalid durable receipt")
	}
	// Replaying identical results returns the same receipt, even after lease expiry.
	if err := db.Model(&model.WorkerDispatch{}).Where("id = ?", dispatch.ID).Update("lease_expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	output := map[string]any{"candidates": []any{map[string]any{"url": "https://example.test/mock-image.png"}}}
	replayed, err := runtime.Complete(ctx, dispatch.ID, dispatch.Attempt, receipt.IdempotencyKey, output)
	if err != nil || replayed.ID != receipt.ID {
		t.Fatalf("result replay: %v", err)
	}
	if _, err := runtime.Fail(ctx, dispatch.ID, dispatch.Attempt, "conflicting-result", "conflict"); !errors.Is(err, jobdomain.ErrDispatchConflict) {
		t.Fatalf("opposite terminal result accepted: %v", err)
	}
	if err := flow.SubmitReceipt(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	for {
		execution, err := api.GetWorkflow(ctx, workflowID)
		if err != nil {
			t.Fatal(err)
		}
		if execution.Status == "COMPLETED" {
			if len(execution.Tasks) != 1 || execution.Tasks[0].OutputData["candidates"] == nil {
				t.Fatal("provider result lost")
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("workflow did not complete")
		case <-ticker.C:
		}
	}
	pending, err := repo.ListPendingResults(ctx, 10)
	if err != nil || len(pending) != 0 {
		t.Fatal("submitted receipt still pending")
	}
}

type resultEngine struct {
	jobdomain.CapabilityTaskEngine
	fail   bool
	result jobdomain.CapabilityTaskResult
}

func (e *resultEngine) UpdateCapabilityTask(_ context.Context, result jobdomain.CapabilityTaskResult) error {
	if e.fail {
		return errors.New("conductor unavailable")
	}
	e.result = result
	return nil
}

func TestDispatchInboxConcurrentReplayAndDeliveryRetry(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	job := &model.Job{ID: "job-test", Scope: "e2e", WorkflowID: "wf-test", WorkflowVersion: 1,
		Input: datatypes.JSON(`{}`), Status: "running", RetryHistory: datatypes.JSON(`[]`), SubmittedBy: "owner", SubmittedAt: time.Now()}
	if err := db.Create(job).Error; err != nil {
		t.Fatal(err)
	}
	step := &model.StepRun{ID: "step-test", JobID: job.ID, StepKey: "image", StepOrder: 1, TemplateID: "e2e", TemplateVersion: 1, Status: "pending"}
	if err := db.Create(step).Error; err != nil {
		t.Fatal(err)
	}
	repo := jobrepo.NewDispatchRepo(db)
	dispatch, err := repo.Create(ctx, &jobdomain.WorkerDispatch{
		ID: "dispatch-test", JobID: job.ID, StepRunID: step.ID, ConductorTaskID: "task-test", ConductorWorkflowID: "engine-test",
		Attempt: 1, CapabilityKey: "image", CapabilityVersion: 1, UserID: "owner", ActorID: "Worker:server", AuthSource: "internal_runtime",
		WorkerInstanceID: "instance", Status: jobdomain.DispatchDelivered, InputJSON: []byte(`{}`), LeaseExpiresAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	receipts := make(chan *jobdomain.WorkerResultReceipt, 8)
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			receipt, err := repo.SaveResult(ctx, dispatch.ID, 1, "complete", "result-1", []byte(`{"candidates":[{"content":"test"}]}`), "digest-1", time.Now())
			if err != nil {
				t.Error(err)
				return
			}
			receipts <- receipt
		}()
	}
	workers.Wait()
	close(receipts)
	var id string
	var count int
	for receipt := range receipts {
		if id != "" && id != receipt.ID {
			t.Fatal("concurrent result created distinct receipts")
		}
		id = receipt.ID
		count++
	}
	if count != 8 {
		t.Fatal("replay requests failed")
	}
	if _, err := repo.SaveResult(ctx, dispatch.ID, 1, "fail", "other-key", []byte(`{"reason":"failed"}`), "digest-2", time.Now()); !errors.Is(err, jobdomain.ErrDispatchConflict) {
		t.Fatalf("opposite result accepted: %v", err)
	}
	engine := &resultEngine{fail: true}
	flow := &worker_gateway.Flow{Dispatches: repo, Engine: engine}
	pending, err := repo.ListPendingResults(ctx, 20)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %v", err)
	}
	if err := flow.SubmitReceipt(ctx, pending[0]); err == nil {
		t.Fatal("unavailable conductor acknowledged")
	}
	pending, err = repo.ListPendingResults(ctx, 20)
	if err != nil || len(pending) != 1 || pending[0].ConductorStatus != "failed" {
		t.Fatal("failed delivery lost durable receipt")
	}
	engine.fail = false
	if err := flow.SubmitReceipt(ctx, pending[0]); err != nil {
		t.Fatal(err)
	}
	pending, err = repo.ListPendingResults(ctx, 20)
	if err != nil || len(pending) != 0 || engine.result.Status != "COMPLETED" || engine.result.Output["candidates"] == nil {
		t.Fatal("retry did not deliver stored result")
	}
}

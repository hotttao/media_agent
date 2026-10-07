//go:build e2e

package worker_gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
)

// Actual native task delivery; local Job persistence is a controlled port.
// No user Workflow, model, external client, token or production DB is touched.
func TestNativeTaskDeliveryIgnoresFailedProjection(t *testing.T) {
	address := strings.TrimRight(os.Getenv("MEDIA_E2E_CONDUCTOR_URL"), "/")
	if address == "" {
		t.Skip("explicit Conductor contract gate")
	}
	var nonce [10]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal("technical fixture identity unavailable")
	}
	name := "retry_delivery_" + hex.EncodeToString(nonce[:])
	taskName := name + "_task"
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	api := conductor.NewClient(address)
	t.Cleanup(func() {
		client := &http.Client{Timeout: 5 * time.Second}
		for _, path := range []string{"/api/metadata/workflow/" + name + "/1", "/api/metadata/taskdefs/" + taskName} {
			req, _ := http.NewRequest(http.MethodDelete, address+path, nil)
			response, err := client.Do(req)
			if err == nil {
				response.Body.Close()
			}
		}
	})
	if api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{{Name: taskName, RetryCount: 1, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutSeconds: 60, ResponseTimeoutSeconds: 30, TimeoutPolicy: "TIME_OUT_WF"}}) != nil {
		t.Fatal("technical task registration failed")
	}
	f, d, _, _, cmd := retryProjectionFixture()
	c := f.Capabilities.(*capabilityRepoStub).item
	c.ConductorTaskDef = taskName
	input := map[string]any{"job_id": "job-1", "step_key": "generate", "capability_key": c.Key, "capability_version": c.Version}
	definition := compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: taskName, TaskReferenceName: "invoke", Type: "SIMPLE", InputParameters: input}}}
	if api.RegisterWorkflow(ctx, definition) != nil {
		t.Fatal("technical workflow registration failed")
	}
	id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: name, Version: 1, Input: map[string]any{}, TaskToDomain: map[string]string{taskName: jobdomain.ExecutionDomainForUser(cmd.UserID)}})
	if err != nil || id == "" {
		t.Fatal("technical Start outcome unconfirmed; no resubmission")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = api.TerminateWorkflow(cleanup, id, "technical delivery gate cleanup")
	})
	engine := &conductor.ExecutionEngine{Client: api}
	f.Engine = engine
	f.Executions = engine
	f.Jobs.(*jobRepoStub).item.ConductorWorkflowID = id
	got, err := f.Poll(ctx, cmd)
	if err != nil || got == nil || d.createCalls != 1 || f.Jobs.(*jobRepoStub).item.Status != jobdomain.JobFailed {
		t.Fatal("native running task blocked by failed projection", err)
	}
	var execution *jobdomain.EngineExecution
	receipt, err := f.Complete(ctx, CompleteCommand{Principal: cmd.Principal, DispatchID: got.ID, WorkerInstanceID: cmd.WorkerInstanceID, Attempt: got.Attempt, IdempotencyKey: name + ":complete", Output: map[string]any{"candidates": []any{map[string]any{"content": "technical fixture result"}}}})
	if err != nil || f.SubmitReceipt(ctx, receipt) != nil {
		t.Fatal("normal result protocol did not complete the technical task")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		execution, err = engine.Get(ctx, id)
		if err == nil && execution.Status == "COMPLETED" && len(execution.Tasks) == 1 {
			t.Log("native running task delivered despite cached failure; no callback or local state reset")
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("completion observation expired")
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("technical workflow did not complete")
}

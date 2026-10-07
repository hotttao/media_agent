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

// A native-engine contract gate: no real vendor, production DB or user Job.
// The result acceptance port is controlled; retry scheduling is real Conductor.
func TestProviderFailureClassificationNativeRetry(t *testing.T) {
	address := strings.TrimRight(os.Getenv("MEDIA_E2E_CONDUCTOR_URL"), "/")
	if address == "" {
		t.Skip("explicit Conductor contract gate")
	}
	for _, kind := range []string{FailureTransient, FailurePermanent} {
		t.Run(kind, func(t *testing.T) {
			var nonce [10]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				t.Fatal("fixture identity unavailable")
			}
			name := "provider_failure_" + hex.EncodeToString(nonce[:])
			taskName := name + "_task"
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			api := conductor.NewClient(address)
			t.Cleanup(func() {
				client := &http.Client{Timeout: 5 * time.Second}
				for _, path := range []string{"/api/metadata/workflow/" + name + "/1", "/api/metadata/taskdefs/" + taskName} {
					req, _ := http.NewRequest(http.MethodDelete, address+path, nil)
					if response, err := client.Do(req); err == nil {
						response.Body.Close()
					}
				}
			})
			if err := api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{{Name: taskName, RetryCount: 1, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutSeconds: 40, ResponseTimeoutSeconds: 20, TimeoutPolicy: "TIME_OUT_WF"}}); err != nil {
				t.Fatal("technical Task registration failed")
			}
			flow, _, _, _, cmd := retryProjectionFixture()
			capability := flow.Capabilities.(*capabilityRepoStub).item
			capability.ConductorTaskDef = taskName
			input := map[string]any{"job_id": "job-1", "step_key": "generate", "capability_key": capability.Key, "capability_version": capability.Version}
			if err := api.RegisterWorkflow(ctx, compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: taskName, TaskReferenceName: "invoke", Type: "SIMPLE", InputParameters: input}}}); err != nil {
				t.Fatal("technical Workflow registration failed")
			}
			id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: name, Version: 1, Input: map[string]any{}, TaskToDomain: map[string]string{taskName: jobdomain.ExecutionDomainForUser(cmd.UserID)}})
			if err != nil || id == "" {
				t.Fatal("Start unconfirmed; no resubmission")
			}
			t.Cleanup(func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = api.TerminateWorkflow(cleanup, id, "technical failure gate cleanup")
			})
			engine := &conductor.ExecutionEngine{Client: api}
			flow.Engine, flow.Executions = engine, engine
			flow.Jobs.(*jobRepoStub).item.ConductorWorkflowID = id
			dispatch, err := flow.Poll(ctx, cmd)
			if err != nil || dispatch == nil {
				t.Fatal("initial Task not delivered")
			}
			receipt, err := flow.Fail(ctx, FailCommand{Principal: cmd.Principal, DispatchID: dispatch.ID, WorkerInstanceID: cmd.WorkerInstanceID, Attempt: dispatch.Attempt, IdempotencyKey: name + ":fail", Reason: "provider unavailable", FailureKind: kind})
			if err != nil || receipt.ConductorStatus != "submitted" {
				t.Fatal("classified failure not delivered")
			}
			// Poll via the normal proxy. A fresh per-dispatch in-memory receipt
			// port represents the new native attempt, not any execution reset.
			for ctx.Err() == nil {
				execution, err := engine.Get(ctx, id)
				if err != nil {
					t.Fatal("execution observation failed")
				}
				if kind == FailurePermanent {
					if execution.Status == "FAILED" {
						if len(execution.Tasks) != 1 || execution.Tasks[0].RetryCount != 0 || execution.Tasks[0].Status != "FAILED_WITH_TERMINAL_ERROR" {
							t.Fatal("permanent failure was retried")
						}
						return
					}
				} else {
					nextFlow := *flow
					nextFlow.Dispatches = &dispatchRepoStub{}
					next, err := nextFlow.Poll(ctx, cmd)
					if err != nil {
						t.Fatal("native retry could not be delivered")
					}
					if next != nil {
						if next.ConductorTaskID == dispatch.ConductorTaskID || next.Attempt != 2 {
							t.Fatal("retry did not use fresh native identity")
						}
						result, err := nextFlow.Complete(ctx, CompleteCommand{Principal: cmd.Principal, DispatchID: next.ID, WorkerInstanceID: cmd.WorkerInstanceID, Attempt: next.Attempt, IdempotencyKey: name + ":complete", Output: map[string]any{"candidates": []any{map[string]any{"content": "technical result"}}}})
						if err != nil || result.ConductorStatus != "submitted" {
							t.Fatal("retried result not delivered")
						}
						if flow.Jobs.(*jobRepoStub).item.Status != jobdomain.JobFailed {
							t.Fatal("proxy reset local execution status")
						}
						for ctx.Err() == nil {
							finished, err := engine.Get(ctx, id)
							if err == nil && finished.Status == "COMPLETED" && len(finished.Tasks) == 2 {
								return
							}
							select {
							case <-ctx.Done():
							case <-time.After(100 * time.Millisecond):
							}
						}
						break
					}
				}
				select {
				case <-ctx.Done():
				case <-time.After(100 * time.Millisecond):
				}
			}
			t.Fatal("native outcome observation expired")
		})
	}
}

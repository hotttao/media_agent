//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared/taskref"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/executable"
)

// Probe the upstream loop contract before changing the business compiler.
// References deliberately contain no "__": Conductor reserves that delimiter
// for iteration suffixes. This is not a gateway regeneration acceptance test.
func TestConductorApprovalRegenerationLoop(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	name := "regen_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	api := conductor.NewClient(address)
	if err := api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{{Name: name, RetryCount: 0, TimeoutSeconds: 120, ResponseTimeoutSeconds: 90, TimeoutPolicy: "TIME_OUT_WF"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, path := range []string{"/api/metadata/workflow/" + name + "/1", "/api/metadata/taskdefs/" + name} {
			req, _ := http.NewRequest(http.MethodDelete, address+path, nil)
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				t.Error("loop definition cleanup failed")
				continue
			}
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				t.Errorf("loop definition cleanup HTTP %d", resp.StatusCode)
			}
		}
	})
	loop := map[string]any{
		"name": "reviewLoop", "taskReferenceName": "reviewLoop", "type": "DO_WHILE",
		"loopCondition": "$.review.decision === 'regenerate'", "inputParameters": map[string]any{},
		"loopOver": []any{
			map[string]any{"name": name, "taskReferenceName": "invoke", "type": "SIMPLE", "inputParameters": map[string]any{}},
			map[string]any{"name": "review", "taskReferenceName": "review", "type": "HUMAN", "inputParameters": map[string]any{"candidates": "${invoke.output.candidates}"}},
		},
	}
	definition := map[string]any{
		"name": name, "version": 1, "schemaVersion": 2,
		"tasks": []any{
			map[string]any{"name": "branches", "taskReferenceName": "branches", "type": "FORK_JOIN", "forkTasks": []any{
				[]any{loop},
				[]any{map[string]any{"name": "parallelReview", "taskReferenceName": "parallelReview", "type": "HUMAN"}},
			}},
			map[string]any{"name": "joined", "taskReferenceName": "joined", "type": "JOIN", "joinOn": []string{"reviewLoop", "parallelReview"}},
		},
		"outputParameters": map[string]any{"selected": "${review.output.selected_candidate}", "candidates": "${invoke.output.candidates}"},
	}
	body, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, address+"/api/metadata/workflow", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("loop definition registration HTTP %d", resp.StatusCode)
	}
	id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: name, Version: 1, Input: map[string]any{}, TaskToDomain: map[string]string{name: name}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		execution, err := api.GetWorkflow(context.Background(), id)
		if err == nil && execution.Status == "RUNNING" {
			_ = api.TerminateWorkflow(context.Background(), id, "isolated loop gate cleanup")
		}
	})
	previousID, parallelID := "", ""
	for iteration := 1; iteration <= 2; iteration++ {
		var invoke *conductor.Task
		for invoke == nil {
			invoke, err = api.Poll(ctx, name, "isolated-loop-test", name)
			if err != nil {
				t.Fatal(err)
			}
		}
		if invoke.TaskID == previousID || invoke.ReferenceTaskName != fmt.Sprintf("invoke__%d", iteration) {
			t.Fatal("loop reused a task identity or an incorrect iteration reference")
		}
		previousID = invoke.TaskID
		candidate := map[string]any{"content": fmt.Sprintf("round_%d", iteration)}
		if err := api.UpdateTask(ctx, conductor.TaskResult{TaskID: invoke.TaskID, WorkflowInstanceID: id, WorkerID: "isolated-loop-test", Status: "COMPLETED", OutputData: map[string]any{"candidates": []any{candidate}}}); err != nil {
			t.Fatal(err)
		}
		for {
			execution, err := api.GetWorkflow(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if execution.Status != "RUNNING" {
				t.Fatalf("loop stopped before approval: %s", execution.Status)
			}
			waiting := false
			for _, task := range execution.Tasks {
				if task.ReferenceTaskName == "parallelReview" {
					if task.Status != "IN_PROGRESS" || (parallelID != "" && parallelID != task.TaskID) {
						t.Fatal("regeneration cancelled or replaced the parallel branch")
					}
					parallelID = task.TaskID
				}
				if task.ReferenceTaskName == fmt.Sprintf("review__%d", iteration) && task.Status == "IN_PROGRESS" {
					encoded, _ := json.Marshal(task.InputData["candidates"])
					if !strings.Contains(string(encoded), fmt.Sprintf("round_%d", iteration)) {
						t.Fatal("review reused candidates from a previous iteration")
					}
					decision := "accept"
					if iteration == 1 {
						decision = "regenerate"
					}
					if err := api.CompleteTaskByReference(ctx, id, task.ReferenceTaskName, map[string]any{"decision": decision, "selected_candidate": candidate}); err != nil {
						t.Fatal(err)
					}
					waiting = true
				}
			}
			if waiting {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("loop approval timed out")
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	parallelCompleted := false
	for {
		execution, err := api.GetWorkflow(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if execution.Status == "COMPLETED" {
			selected, _ := execution.Output["selected"].(map[string]any)
			candidates, _ := json.Marshal(execution.Output["candidates"])
			if selected["content"] != "round_2" || !strings.Contains(string(candidates), "round_2") || strings.Contains(string(candidates), "round_1") {
				t.Fatal("downstream reference did not resolve the final iteration")
			}
			break
		}
		for _, task := range execution.Tasks {
			if task.ReferenceTaskName == "reviewLoop" && task.Status == "COMPLETED" && !parallelCompleted {
				if parallelID == "" {
					t.Fatal("parallel branch was not observed")
				}
				if err := api.CompleteTaskByReference(ctx, id, "parallelReview", map[string]any{}); err != nil {
					t.Fatal(err)
				}
				parallelCompleted = true
			}
		}
		if execution.Status != "RUNNING" {
			t.Fatalf("loop execution failed: %s", execution.Status)
		}
		select {
		case <-ctx.Done():
			t.Fatal("loop completion timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// This is a compiler/Conductor protocol gate, not a substitute for a User Job
// or Worker Gateway acceptance test. Only a uniquely named test capability is
// polled directly; existing Renderer/Finalizer workers run the internal tasks.
func TestConductorRuntimeCandidateApproval(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL and run the internal worker")
	}
	for _, tc := range []struct {
		name, policy     string
		count            int
		review           bool
		nextCount        int
		retryFailedRound bool
	}{
		{"single automatic", "when_multiple_candidates", 1, false, 0, false},
		{"unexpected multiple", "when_multiple_candidates", 2, true, 0, false},
		{"single always", "always", 1, true, 0, false},
		{"regenerate then approve", "when_multiple_candidates", 2, true, 2, false},
		{"regenerate then automatic", "when_multiple_candidates", 2, true, 1, false},
		{"retry regenerated capability", "when_multiple_candidates", 2, true, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			name := "review_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			api := conductor.NewClient(address)
			if err := api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{{Name: name, RetryCount: 0, TimeoutSeconds: 120, ResponseTimeoutSeconds: 90, TimeoutPolicy: "TIME_OUT_WF"}}); err != nil {
				t.Fatal(err)
			}
			ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.LegacySchemaVersion, WorkflowID: name, WorkflowVersion: 1, OutputStepKey: "image", Steps: []executable.ExecutableStepIR{{
				Key: "image", Template: executable.TemplateIR{ID: "test-template", Version: 1, Body: "draw a test image"},
				Capability: executable.CapabilityIR{Key: name, Version: 1, ConductorTaskDef: name, ExecutionTarget: "personal_worker"},
				Invocation: template.InvocationMapping{"prompt": {Op: template.InvocationRenderTemplate}}, OutputAssetKind: "model_image", ApprovalPolicy: tc.policy, ApprovalRequired: tc.policy == "always", CandidateCount: 1,
			}}}
			definition, err := compiler.CompileExecutableIR(ir)
			if err != nil {
				t.Fatal(err)
			}
			if err := api.RegisterWorkflow(ctx, definition); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				for _, path := range []string{"/api/metadata/workflow/" + definition.Name + "/1", "/api/metadata/taskdefs/" + name} {
					req, _ := http.NewRequest(http.MethodDelete, address+path, nil)
					resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
					if err != nil {
						t.Error("test definition cleanup failed")
						continue
					}
					resp.Body.Close()
					if resp.StatusCode >= 300 {
						t.Errorf("test definition cleanup HTTP %d", resp.StatusCode)
					}
				}
			})
			id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: definition.Name, Version: 1, Input: map[string]any{"job_id": name}, TaskToDomain: map[string]string{name: name}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				execution, err := api.GetWorkflow(context.Background(), id)
				if err == nil && execution.Status == "RUNNING" {
					_ = api.TerminateWorkflow(context.Background(), id, "isolated approval gate cleanup")
				}
			})
			completeInvoke := func(count, round int) (*conductor.Task, []map[string]any) {
				var invoke *conductor.Task
				for invoke == nil {
					invoke, err = api.Poll(ctx, name, "isolated-approval-test", name)
					if err != nil {
						t.Fatal(err)
					}
				}
				if invoke.ReferenceTaskName != fmt.Sprintf("%s__%d", taskref.Reference("image", "invoke"), round) {
					t.Fatal("capability was delivered from an incorrect generation")
				}
				prompt, _ := invoke.InputData["prompt"].(string)
				if round == 1 && prompt != "draw a test image" {
					t.Fatal("first round reused feedback")
				}
				if round == 2 && prompt != "draw a test image\n\n用户重新生成反馈（本轮）：\n更亮 {{.must_remain_text}}" {
					t.Fatal("second round did not receive previous generation feedback as plain text")
				}
				if round == 2 && tc.retryFailedRound {
					failedID := invoke.TaskID
					if err := api.UpdateTask(ctx, conductor.TaskResult{TaskID: failedID, WorkflowInstanceID: id, WorkerID: "isolated-approval-test", Status: "FAILED_WITH_TERMINAL_ERROR", ReasonForIncompletion: "isolated retry fixture failure"}); err != nil {
						t.Fatal(err)
					}
					var currentRender string
					for {
						execution, err := api.GetWorkflow(ctx, id)
						if err != nil {
							t.Fatal(err)
						}
						if execution.Status == "FAILED" {
							for _, task := range execution.Tasks {
								if task.ReferenceTaskName == taskref.Reference("image", "render")+"__2" {
									currentRender = task.TaskID
								}
							}
							break
						}
						time.Sleep(20 * time.Millisecond)
					}
					if currentRender == "" {
						t.Fatal("latest round renderer missing")
					}
					if err := api.RerunFromTask(ctx, id, currentRender); err != nil {
						t.Fatal(err)
					}
					invoke = nil
					for invoke == nil {
						invoke, err = api.Poll(ctx, name, "isolated-approval-test", name)
						if err != nil {
							t.Fatal(err)
						}
					}
					if invoke.TaskID == failedID || invoke.ReferenceTaskName != taskref.Reference("image", "invoke")+"__2" || invoke.InputData["prompt"] != prompt {
						t.Fatal("retry reused failed capability identity, old generation or lost feedback")
					}
					t.Log("failed second generation retried with a new Capability Task ID and unchanged feedback prompt")
				}
				candidates := make([]map[string]any, count)
				for i := range candidates {
					candidates[i] = map[string]any{"index": i, "content": fmt.Sprintf("round_%d_candidate_%d", round, i)}
				}
				if err := api.UpdateTask(ctx, conductor.TaskResult{TaskID: invoke.TaskID, WorkflowInstanceID: id, WorkerID: "isolated-approval-test", Status: "COMPLETED", OutputData: map[string]any{"candidates": candidates}}); err != nil {
					t.Fatal(err)
				}
				return invoke, candidates
			}
			invoke, candidates := completeInvoke(tc.count, 1)
			approved, regenerated := false, false
			round := 1
			for {
				execution, err := api.GetWorkflow(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if execution.Status == "FAILED" || execution.Status == "TIMED_OUT" || execution.Status == "TERMINATED" {
					t.Fatalf("approval execution failed: %s", execution.Status)
				}
				for _, task := range execution.Tasks {
					if task.ReferenceTaskName != fmt.Sprintf("%s__%d", taskref.Reference("image", "approval"), round) || task.Status != "IN_PROGRESS" {
						continue
					}
					if !tc.review {
						t.Fatal("single automatic candidate incorrectly required approval")
					}
					if !approved {
						if tc.count > 1 {
							// Conductor's rerun operation requires a terminal
							// workflow; a waiting HUMAN task is not terminal.
							err := api.RerunFromTask(ctx, id, invoke.TaskID)
							if err == nil || !strings.Contains(err.Error(), "409") {
								t.Fatal("unexpected Conductor nonterminal rerun contract")
							}
						}
						for _, later := range execution.Tasks {
							if later.ReferenceTaskName == "step__image__finalize" {
								t.Fatal("finalization started before human approval")
							}
						}
						if tc.nextCount > 0 && !regenerated {
							if err := api.CompleteTaskByReference(ctx, id, task.ReferenceTaskName, map[string]any{"decision": "regenerate", "feedback": "更亮 {{.must_remain_text}}"}); err != nil {
								t.Fatal(err)
							}
							firstID := invoke.TaskID
							round = 2
							tc.count = tc.nextCount
							invoke, candidates = completeInvoke(tc.count, round)
							if invoke.TaskID == firstID {
								t.Fatal("regeneration reused the capability task identity")
							}
							regenerated = true
							continue
						}
						if err := api.CompleteTaskByReference(ctx, id, task.ReferenceTaskName, map[string]any{"decision": "accept", "selected_index": tc.count - 1, "selected_candidate": candidates[tc.count-1]}); err != nil {
							t.Fatal(err)
						}
						approved = true
					}
				}
				if execution.Status == "COMPLETED" {
					if (approved || regenerated) != tc.review || (tc.nextCount > 0 && !regenerated) {
						t.Fatal("required human approval was bypassed")
					}
					var output map[string]any
					for _, task := range execution.Tasks {
						if task.ReferenceTaskName == "step__image__finalize" && task.Status == "COMPLETED" {
							output, _ = task.OutputData["output_resource"].(map[string]any)
						}
					}
					if output == nil || output["content"] != fmt.Sprintf("round_%d_candidate_%d", round, tc.count-1) || execution.Output["output_resource_id"] == "" {
						t.Fatal("finalizer did not use the selected candidate")
					}
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("approval gate timed out")
				case <-time.After(200 * time.Millisecond):
				}
			}
		})
	}
}

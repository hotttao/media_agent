//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
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

// Probe native retry against the current compiler's FORK_JOIN + DO_WHILE +
// SWITCH layout. Only SIMPLE task names are renamed into unique test queues;
// references, graph, expressions and loop conditions are compiler output.
// No business database, local Retry command, state reset or restore task is used.
func TestConductorNativeRetryCompiledParallelWorkflow(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL for real Conductor integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	api := conductor.NewClient(address)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	domain := "native_probe_" + suffix
	renderName, invokeName, finalizeName := "native_render_"+suffix, "native_invoke_"+suffix, "native_finalize_"+suffix
	step := func(key string, order int, deps []string) executable.ExecutableStepIR {
		return executable.ExecutableStepIR{Key: key, Order: order, DependsOn: deps, Template: executable.TemplateIR{ID: "fixture", Version: 1, Body: "fixture prompt"}, Context: map[string]executable.ValueRef{}, Capability: executable.CapabilityIR{Key: "fixture", Version: 1, ConductorTaskDef: invokeName, ExecutionTarget: "personal_worker"}, Invocation: template.InvocationMapping{"prompt": {Op: template.InvocationRenderTemplate}}, OutputAssetKind: "image", ApprovalPolicy: "when_multiple_candidates", CandidateCount: 1}
	}
	ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.LegacySchemaVersion, WorkflowID: "native_retry_" + suffix, WorkflowVersion: 1, OutputStepKey: "merge", Steps: []executable.ExecutableStepIR{step("a", 1, nil), step("b", 2, nil), step("merge", 3, []string{"a", "b"})}}
	definition, err := compiler.CompileExecutableIR(ir)
	if err != nil {
		t.Fatal(err)
	}
	var isolate func([]compiler.Task)
	isolate = func(tasks []compiler.Task) {
		for i := range tasks {
			task := &tasks[i]
			if task.Name == compiler.RenderTemplateTaskName {
				task.Name = renderName
			}
			if task.Name == compiler.FinalizeStepTaskName {
				task.Name = finalizeName
			}
			isolate(task.LoopOver)
			for _, branch := range task.ForkTasks {
				isolate(branch)
			}
			for _, branch := range task.DecisionCases {
				isolate(branch)
			}
		}
	}
	isolate(definition.Tasks)
	t.Cleanup(func() {
		for _, path := range []string{"/api/metadata/workflow/" + definition.Name + "/1", "/api/metadata/taskdefs/" + renderName, "/api/metadata/taskdefs/" + invokeName, "/api/metadata/taskdefs/" + finalizeName} {
			req, _ := http.NewRequest(http.MethodDelete, address+path, nil)
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				t.Error(err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				t.Errorf("isolated metadata cleanup: %d", resp.StatusCode)
			}
		}
	})
	var defs []conductor.TaskDefinition
	for _, name := range []string{renderName, invokeName, finalizeName} {
		defs = append(defs, conductor.TaskDefinition{Name: name, RetryCount: 0, TimeoutSeconds: 120, ResponseTimeoutSeconds: 90, TimeoutPolicy: "TIME_OUT_WF"})
	}
	if err := api.RegisterTaskDefinitions(ctx, defs); err != nil {
		t.Fatal(err)
	}
	if err := api.RegisterWorkflow(ctx, definition); err != nil {
		t.Fatal(err)
	}
	id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: definition.Name, Version: 1, Input: map[string]any{"job_id": ir.WorkflowID, "inputs": map[string]any{}}, TaskToDomain: map[string]string{renderName: domain, invokeName: domain, finalizeName: domain}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = api.TerminateWorkflow(context.Background(), id, "isolated native retry probe cleanup") })
	poll := func(name string) *conductor.Task {
		t.Helper()
		for {
			task, err := api.Poll(ctx, name, "native-retry-probe", domain)
			if err == nil && task != nil {
				return task
			}
			if err != nil || ctx.Err() != nil {
				if e, readErr := api.GetWorkflow(ctx, id); readErr == nil {
					t.Logf("isolated native probe state: %s", e.Status)
					for _, observed := range e.Tasks {
						t.Logf("task %s type=%s status=%s retry=%d", observed.ReferenceTaskName, observed.TaskType, observed.Status, observed.RetryCount)
					}
				}
				t.Fatalf("native probe queue missing task: %v", err)
			}
			// An empty bounded poll is not a failed workflow. Native JOIN retry
			// may converge on the asynchronous decider; keep observing this run.
			if e, readErr := api.GetWorkflow(ctx, id); readErr != nil || e.Status == "FAILED" || e.Status == "TIMED_OUT" || e.Status == "TERMINATED" {
				t.Fatal("native probe execution terminated while waiting for its task")
			}
		}
	}
	finish := func(task *conductor.Task, status string, output map[string]any) {
		t.Helper()
		if err := api.UpdateTask(ctx, conductor.TaskResult{WorkflowInstanceID: id, TaskID: task.TaskID, WorkerID: "native-retry-probe", Status: status, OutputData: output}); err != nil {
			t.Fatal(err)
		}
	}
	candidates := map[string]any{"candidates": []any{map[string]any{"index": 0, "url": "fixture://image"}}}
	wait := func(status string) *conductor.WorkflowExecution {
		t.Helper()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			e, err := api.GetWorkflow(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if e.Status == status {
				return e
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				t.Fatalf("native probe expected %s, last status %s", status, e.Status)
				return nil
			}
		}
	}
	for i := 0; i < 2; i++ {
		render := poll(renderName)
		finish(render, "COMPLETED", map[string]any{"prompt": "fixture prompt"})
	}
	var failedInvoke *conductor.Task
	for i := 0; i < 2; i++ {
		invoke := poll(invokeName)
		switch invoke.InputData["step_key"] {
		case "a":
			failedInvoke = invoke
		case "b":
			finish(invoke, "COMPLETED", candidates)
		default:
			t.Fatal("unexpected first-wave capability")
		}
	}
	if failedInvoke == nil {
		t.Fatal("branch a invoke missing")
	}
	bFinal := poll(finalizeName)
	if bFinal.InputData["step_key"] != "b" {
		t.Fatal("completed branch b finalizer missing")
	}
	finish(bFinal, "COMPLETED", map[string]any{"output_resource_id": "asset_b"})
	finish(failedInvoke, "FAILED", nil)
	before := wait("FAILED")
	retained := map[string][]byte{}
	for _, task := range before.Tasks {
		if strings.HasPrefix(task.ReferenceTaskName, "step__b__") || strings.HasPrefix(task.ReferenceTaskName, taskref.Reference("b", "render")) || strings.HasPrefix(task.ReferenceTaskName, taskref.Reference("b", "invoke")) || strings.HasPrefix(task.ReferenceTaskName, taskref.Reference("b", "loop")) || task.TaskID == bFinal.TaskID {
			if task.Status != "COMPLETED" {
				t.Fatal("branch b was not completed before failure")
			}
			retained[task.TaskID], _ = json.Marshal(task)
		}
	}
	if len(retained) < 4 {
		t.Fatal("insufficient completed branch evidence")
	}
	if err := api.RetryWorkflow(ctx, id); err != nil {
		t.Fatal(err)
	}
	retried := poll(invokeName)
	if retried.InputData["step_key"] != "a" || retried.TaskID == failedInvoke.TaskID || retried.InputData["prompt"] != failedInvoke.InputData["prompt"] {
		t.Fatal("native retry did not resume the failed invocation with existing prompt")
	}
	finish(retried, "COMPLETED", candidates)
	aFinal := poll(finalizeName)
	if aFinal.InputData["step_key"] != "a" {
		t.Fatal("branch a finalizer missing after retry")
	}
	finish(aFinal, "COMPLETED", map[string]any{"output_resource_id": "asset_a"})
	mergeRender := poll(renderName)
	if !strings.HasPrefix(mergeRender.ReferenceTaskName, taskref.Reference("merge", "render")) {
		t.Fatal("retry rerendered a completed branch instead of merge")
	}
	finish(mergeRender, "COMPLETED", map[string]any{"prompt": "fixture prompt"})
	mergeInvoke := poll(invokeName)
	if mergeInvoke.InputData["step_key"] != "merge" {
		t.Fatal("unexpected resumed capability")
	}
	finish(mergeInvoke, "COMPLETED", candidates)
	mergeFinal := poll(finalizeName)
	if mergeFinal.InputData["step_key"] != "merge" {
		t.Fatal("merge finalizer missing")
	}
	finish(mergeFinal, "COMPLETED", map[string]any{"output_resource_id": "asset_merge"})
	after := wait("COMPLETED")
	aRenderCount := 0
	for _, task := range after.Tasks {
		if strings.HasPrefix(task.ReferenceTaskName, taskref.Reference("a", "render")) {
			aRenderCount++
		}
		if original, ok := retained[task.TaskID]; ok {
			current, _ := json.Marshal(task)
			if !bytes.Equal(original, current) {
				t.Fatal("native retry changed a completed parallel task")
			}
			delete(retained, task.TaskID)
		}
	}
	if len(retained) != 0 || aRenderCount != 1 || after.Output["output_resource_id"] != "asset_merge" {
		t.Fatal("native retry lost completed work or failed to converge")
	}
}

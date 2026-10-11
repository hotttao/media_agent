//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	capability "media_agent/media_workflow/biz/domain/capability"
	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/executable"
	"media_agent/media_workflow/biz/workflow/finalize_task"
	"media_agent/media_workflow/biz/workflow/render_task"
	"media_agent/media_workflow/biz/workflow/reuse_task"
)

// This gate exercises the real engine's compiled branch/loop and expression
// resolution. Task names are isolated aliases; source authorization/persistence
// is tested separately through the real handlers and PostgreSQL repository.
func TestConductorCompiledReuseBranch(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	for _, allHit := range []bool{true, false} {
		t.Run(fmt.Sprintf("all_hit_%v", allHit), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			name := "reuse_gate_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			aliases := map[string]string{render_task.TaskDefinitionName: name + "_render", reuse_task.TaskDefinitionName: name + "_check", finalize_task.TaskDefinitionName: name + "_finalize"}
			provider := name + "_provider"
			step := executable.ExecutableStepIR{Key: "first", Template: executable.TemplateIR{ID: "tpl", Version: 1, Body: "first"}, Capability: executable.CapabilityIR{Key: "cap", Version: 1, ConductorTaskDef: provider, Inputs: []capability.Input{{Name: "prompt", Type: capability.InputString, Required: true}}}, Invocation: template.InvocationMapping{"prompt": {Op: template.InvocationRenderTemplate}}, CandidateCount: 1, OutputAssetKind: "text", ApprovalPolicy: "always", ReusePolicy: "reuse_if_available"}
			second := step
			second.Key, second.Template.Body, second.DependsOn = "second", "downstream {{.prior.content}}", []string{"first"}
			second.Context = map[string]executable.ValueRef{"prior": {Source: executable.ValueStepOutput, Ref: "first"}}
			parallel := step
			parallel.Key, parallel.Template.Body = "parallel", "parallel fixture"
			second.DependsOn = []string{"first", "parallel"}
			definition, err := compiler.CompileExecutableIR(executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: name, WorkflowVersion: 1, OutputStepKey: "second", Steps: []executable.ExecutableStepIR{step, parallel, second}})
			if err != nil {
				t.Fatal(err)
			}
			var rename func([]compiler.Task)
			rename = func(tasks []compiler.Task) {
				for i := range tasks {
					if alias := aliases[tasks[i].Name]; alias != "" {
						tasks[i].Name = alias
					}
					rename(tasks[i].LoopOver)
					for _, branch := range tasks[i].DecisionCases {
						rename(branch)
					}
					for _, branch := range tasks[i].ForkTasks {
						rename(branch)
					}
				}
			}
			rename(definition.Tasks)
			api := conductor.NewClient(address)
			taskDefs := []conductor.TaskDefinition{}
			for _, alias := range append([]string{provider}, aliases[render_task.TaskDefinitionName], aliases[reuse_task.TaskDefinitionName], aliases[finalize_task.TaskDefinitionName]) {
				taskDefs = append(taskDefs, conductor.TaskDefinition{Name: alias, RetryCount: 0, TimeoutSeconds: 120, ResponseTimeoutSeconds: 90, TimeoutPolicy: "TIME_OUT_WF"})
			}
			if err := api.RegisterTaskDefinitions(ctx, taskDefs); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				paths := []string{"/api/metadata/workflow/" + definition.Name + "/1"}
				for _, def := range taskDefs {
					paths = append(paths, "/api/metadata/taskdefs/"+def.Name)
				}
				for _, path := range paths {
					req, _ := http.NewRequest(http.MethodDelete, address+path, nil)
					resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
					if err != nil {
						t.Error("isolated definition cleanup failed")
						continue
					}
					resp.Body.Close()
					if resp.StatusCode >= 300 {
						t.Errorf("isolated definition cleanup HTTP %d", resp.StatusCode)
					}
				}
			})
			if err := api.RegisterWorkflow(ctx, definition); err != nil {
				t.Fatal(err)
			}
			id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: definition.Name, Version: 1, Input: map[string]any{"job_id": name}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = api.TerminateWorkflow(context.Background(), id, "isolated reuse gate cleanup") })
			invokes, reviews := 0, 0
			failedOnce := false
			decode := func(raw any, target any) {
				data, err := json.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, target); err != nil {
					t.Fatal(err)
				}
			}
			for {
				snapshot, err := api.GetWorkflow(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if snapshot.Status == "COMPLETED" {
					want := 0
					if !allHit {
						want = 2
					}
					if invokes != want || reviews != want || snapshot.Output["output_resource_id"] != "gen_second" {
						t.Fatalf("incorrect compiled branch results: invokes=%d reviews=%d output=%v", invokes, reviews, snapshot.Output)
					}
					break
				}
				if snapshot.Status != "RUNNING" {
					t.Fatalf("compiled reuse execution stopped: %s", snapshot.Status)
				}
				for _, native := range snapshot.Tasks {
					if native.Status != "SCHEDULED" && native.Status != "IN_PROGRESS" {
						continue
					}
					if native.TaskType == "HUMAN" {
						reviews++
						output := map[string]any{"decision": "regenerate", "feedback": "brighter"}
						if reviews == 2 {
							output = map[string]any{"decision": "accept", "selected_candidate": map[string]any{"content": "accepted new"}}
						}
						if err := api.UpdateTask(ctx, conductor.TaskResult{TaskID: native.TaskID, WorkflowInstanceID: id, Status: "COMPLETED", OutputData: output}); err != nil {
							t.Fatal(err)
						}
						continue
					}
					if native.TaskType != provider && !strings.HasPrefix(native.TaskType, name+"_") {
						continue
					}
					task, err := api.Poll(ctx, native.TaskType, name, "")
					if err != nil || task == nil {
						t.Fatalf("isolated task poll: %v", err)
					}
					var output any
					switch task.TaskType {
					case aliases[render_task.TaskDefinitionName]:
						var input render_task.Input
						decode(task.InputData, &input)
						output, err = (render_task.Handler{}).Execute(input)
					case aliases[reuse_task.TaskDefinitionName]:
						var input reuse_task.Input
						decode(task.InputData, &input)
						if input.StepKey == "second" && input.ProviderInput["prompt"] != "downstream accepted first" {
							t.Fatalf("downstream did not receive reused source: %v", input.ProviderInput)
						}
						output = reuse_task.Output{Hit: input.StepKey != "second" || allHit, Resource: executable.StepResource{ID: "gen_" + input.StepKey, Content: "accepted " + input.StepKey, AssetKind: "text"}}
					case provider:
						if !allHit && !failedOnce {
							failedOnce = true
							preserved := map[string]bool{}
							for _, completed := range snapshot.Tasks {
								if completed.Status == "COMPLETED" && !completed.Retried {
									preserved[completed.TaskID] = false
								}
							}
							if err := api.UpdateTask(ctx, conductor.TaskResult{TaskID: task.TaskID, WorkflowInstanceID: id, Status: "FAILED_WITH_TERMINAL_ERROR", ReasonForIncompletion: "isolated native retry gate"}); err != nil {
								t.Fatal(err)
							}
							for {
								failed, err := api.GetWorkflow(ctx, id)
								if err != nil {
									t.Fatal(err)
								}
								if failed.Status == "FAILED" {
									break
								}
								select {
								case <-ctx.Done():
									t.Fatal("failure projection timed out")
								case <-time.After(100 * time.Millisecond):
								}
							}
							if err := api.RetryWorkflow(ctx, id); err != nil {
								t.Fatal(err)
							}
							resumed, err := api.GetWorkflow(ctx, id)
							if err != nil {
								t.Fatal(err)
							}
							for _, completed := range resumed.Tasks {
								if _, exists := preserved[completed.TaskID]; exists && completed.Status == "COMPLETED" && !completed.Retried {
									preserved[completed.TaskID] = true
								}
							}
							for _, exists := range preserved {
								if !exists {
									t.Fatal("native retry replaced an already completed reuse/check branch")
								}
							}
							continue
						}
						invokes++
						output = map[string]any{"candidates": []map[string]any{{"content": "accepted new"}}}
						if invokes == 2 && !strings.Contains(fmt.Sprint(task.InputData["prompt"]), "brighter") {
							t.Fatal("regeneration lost feedback")
						}
					case aliases[finalize_task.TaskDefinitionName]:
						var input finalize_task.Input
						decode(task.InputData, &input)
						if input.ReuseResult == nil {
							t.Fatal("missing native reuse output")
						}
						if input.ReuseResult.Hit && (len(input.Candidates) != 0 || input.SelectedCandidate != nil) {
							t.Fatal("hit resolved nonexistent generation as real input")
						}
						output = executable.StepOutput{OutputResourceID: "gen_" + input.StepKey, OutputResource: input.ReuseResult.Resource}
					}
					if err != nil {
						t.Fatal(err)
					}
					var result map[string]any
					decode(output, &result)
					if err := api.UpdateTask(ctx, conductor.TaskResult{TaskID: task.TaskID, WorkflowInstanceID: id, Status: "COMPLETED", OutputData: result}); err != nil {
						t.Fatal(err)
					}
				}
				select {
				case <-ctx.Done():
					t.Fatal("compiled reuse gate timed out")
				case <-time.After(100 * time.Millisecond):
				}
			}
		})
	}
}

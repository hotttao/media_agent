//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	domain "media_agent/media_workflow/biz/domain/job"
	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/executable"
	"media_agent/media_workflow/biz/workflow/finalize_task"
	"media_agent/media_workflow/biz/workflow/render_task"
)

type finalizeJobPort struct{ item *domain.Job }

func (p finalizeJobPort) Get(context.Context, string) (*domain.Job, error) { return p.item, nil }

type finalizeStepPort struct{ item *domain.StepRun }

func (p finalizeStepPort) GetByJobAndStepKey(context.Context, string, string) (*domain.StepRun, error) {
	return p.item, nil
}

// Actual compiler graph, Runner, Finalize handler, HTTP registration client and
// Conductor automatic retry. Materials is an explicit protocol stand-in, not
// proof of its PostgreSQL persistence or real storage. No Job API GET is called.
func TestFinalizeNativeAutomaticRetryAfterRegistrationResponseLoss(t *testing.T) {
	address := strings.TrimRight(os.Getenv("MEDIA_E2E_CONDUCTOR_URL"), "/")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL for real Conductor integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// Read only the shared Finalize task definition; never mutate or delete it.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, address+"/api/metadata/taskdefs/"+finalize_task.TaskDefinitionName, nil)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal("Finalize metadata preflight unavailable")
	}
	var registered conductor.TaskDefinition
	decodeErr := json.NewDecoder(response.Body).Decode(&registered)
	response.Body.Close()
	if response.StatusCode != 200 || decodeErr != nil || registered.RetryCount < 1 {
		t.Fatal("preexisting Finalize definition must enable native retry")
	}

	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "finalize-test-internal")
	var lock sync.Mutex
	registrations := map[string]string{}
	calls, invalid := 0, false
	assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var registration client.UpsertGenAssetReq
		if r.Method != "POST" || r.URL.Path != "/api/internal/v1/generated-assets:upsert" || r.Header.Get("X-Media-Materials-Internal-Token") != "finalize-test-internal" || json.NewDecoder(r.Body).Decode(&registration) != nil || registration.ResultKey == "" || registration.SourceProductID != "test-product" {
			lock.Lock()
			invalid = true
			lock.Unlock()
			w.WriteHeader(400)
			return
		}
		lock.Lock()
		calls++
		first := calls == 1
		id := registrations[registration.ResultKey]
		if id == "" {
			id = "gen_protocol_" + registration.ResultKey[:12]
			registrations[registration.ResultKey] = id
		}
		lock.Unlock()
		if first {
			// Commit the stable registration, then drop the TCP response.
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	}))
	defer assets.Close()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	queueDomain := "finalize_native_" + suffix
	renderName, invokeName := "finalize_render_"+suffix, "finalize_invoke_"+suffix
	ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: "finalize_flow_" + suffix, WorkflowVersion: 1, OutputStepKey: "image", Steps: []executable.ExecutableStepIR{{
		Key: "image", Order: 1, Template: executable.TemplateIR{ID: "test-template", Version: 1, Body: "technical fixture"}, Context: map[string]executable.ValueRef{},
		Capability: executable.CapabilityIR{Key: "test-capability", Version: 1, ConductorTaskDef: invokeName, ExecutionTarget: "personal_worker"},
		Invocation: template.InvocationMapping{"prompt": {Op: template.InvocationRenderTemplate}}, OutputAssetKind: "image", ApprovalPolicy: "when_multiple_candidates", CandidateCount: 1,
	}}}
	definition, err := compiler.CompileExecutableIR(ir)
	if err != nil {
		t.Fatal(err)
	}
	// Only Render is aliased; Finalize uses the actual production handler/type.
	definition.Tasks[0].LoopOver[0].Name = renderName
	api := conductor.NewClient(address)
	if err := api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{{Name: renderName, RetryCount: 0, TimeoutSeconds: 120, ResponseTimeoutSeconds: 60, TimeoutPolicy: "TIME_OUT_WF"}, {Name: invokeName, RetryCount: 0, TimeoutSeconds: 120, ResponseTimeoutSeconds: 60, TimeoutPolicy: "TIME_OUT_WF"}}); err != nil {
		t.Fatal("isolated task registration failed")
	}
	t.Cleanup(func() {
		for _, endpoint := range []string{"/api/metadata/workflow/" + definition.Name + "/1", "/api/metadata/taskdefs/" + renderName, "/api/metadata/taskdefs/" + invokeName} {
			req, _ := http.NewRequest(http.MethodDelete, address+endpoint, nil)
			if response, err := (&http.Client{Timeout: 5 * time.Second}).Do(req); err == nil {
				response.Body.Close()
			}
		}
	})
	if err := api.RegisterWorkflow(ctx, definition); err != nil {
		t.Fatal("isolated definition registration failed")
	}
	jobID := "finalize_job_" + suffix
	id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: definition.Name, Version: 1, Input: map[string]any{"job_id": jobID, "inputs": map[string]any{}}, TaskToDomain: map[string]string{renderName: queueDomain, invokeName: queueDomain, finalize_task.TaskDefinitionName: queueDomain}})
	if err != nil {
		t.Fatal("isolated Start outcome unconfirmed; do not resubmit")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.TerminateWorkflow(cleanup, id, "Finalize technical gate cleanup")
	})
	job := &domain.Job{ID: jobID, OrganizationID: "test-org", Scope: "ai_clothing", SubmittedBy: "test-owner", ConductorWorkflowID: id, Status: domain.JobPending, Input: domain.JobInput{Product: &domain.ProductData{ID: "test-product"}}}
	step := &domain.StepRun{ID: "finalize_step_" + suffix, JobID: jobID, StepKey: "image", TemplateID: "test-template", TemplateVersion: 1, AssetKind: "image"}
	handler := finalize_task.Handler{Jobs: finalizeJobPort{job}, Steps: finalizeStepPort{step}, Executions: &conductor.ExecutionEngine{Client: api}, Assets: client.NewMaterialsClient(assets.URL), StorageOrigin: "https://storage.test"}
	runner := conductor.Runner{Client: api, WorkerID: queueDomain, TaskDomains: map[string]string{renderName: queueDomain, invokeName: queueDomain, finalize_task.TaskDefinitionName: queueDomain}, Handlers: map[string]conductor.Handler{
		renderName: conductor.JSONHandler((render_task.Handler{}).Execute),
		invokeName: func(context.Context, map[string]any) (map[string]any, error) {
			return map[string]any{"candidates": []domain.Candidate{{URL: "https://storage.test/media/materials/test-org/ai_clothing/fixture.png"}}}, nil
		},
		finalize_task.TaskDefinitionName: conductor.JSONHandlerContext(handler.Execute),
	}}
	workerCtx, stopWorker := context.WithCancel(ctx)
	stopped := make(chan error, 1)
	go func() { stopped <- runner.Run(workerCtx) }()
	defer func() { stopWorker(); <-stopped }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		execution, err := api.GetWorkflow(ctx, id)
		if err != nil {
			t.Fatal("isolated workflow observation unavailable")
		}
		if execution.Status == "COMPLETED" {
			lock.Lock()
			numCalls, numAssets, rejected := calls, len(registrations), invalid
			lock.Unlock()
			if numCalls != 2 || numAssets != 1 || rejected || !strings.HasPrefix(execution.Output["output_resource_id"].(string), "gen_protocol_") {
				t.Fatal("native retry lost registration identity")
			}
			var failed, successful int
			for _, task := range execution.Tasks {
				if task.TaskType != finalize_task.TaskDefinitionName {
					continue
				}
				if task.Status == "FAILED" && task.Retried {
					failed++
				}
				if task.Status == "COMPLETED" && task.RetryCount == 1 {
					successful++
				}
			}
			if failed != 1 || successful != 1 || job.Status != domain.JobPending || step.OutputAssetID != "" {
				t.Fatal("workflow needed local state changes or did not use native retry")
			}
			t.Log("compiled workflow completed with no Job GET; native automatic retry replayed one registered Asset")
			return
		}
		if execution.Status == "FAILED" || execution.Status == "TIMED_OUT" || execution.Status == "TERMINATED" {
			t.Fatalf("isolated workflow terminated: %s", execution.Status)
		}
		select {
		case <-ctx.Done():
			t.Fatal("observe the same isolated instance: completion timeout")
		case <-ticker.C:
		}
	}
}

//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	service "media_agent/media_workflow/biz/service/job"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/executable"
)

// Native rerun + isolated PostgreSQL, with a transport failure after the real
// engine commit. The simple HUMAN graph isolates the adapter contract; this is
// not the public four-node Capability/Worker business acceptance gate.
func TestNativeInputRerunRecoversLostResponseWithoutSecondPost(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	db := newDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	name := "input_rerun_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	api := conductor.NewClient(address)
	if err := api.RegisterWorkflow(ctx, compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: "hold", TaskReferenceName: "hold", Type: "HUMAN"}}}); err != nil {
		t.Fatal("isolated definition registration failed")
	}
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, address+"/api/metadata/workflow/"+name+"/1", nil)
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Error("isolated definition cleanup failed")
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			t.Error("isolated definition cleanup rejected")
		}
	})
	oldInput := map[string]any{"job_id": name, "inputs": map[string]any{"old": "fixture input"}}
	id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: name, Version: 1, Input: oldInput})
	if err != nil {
		t.Fatal("isolated native start failed")
	}
	t.Cleanup(func() { _ = api.TerminateWorkflow(context.Background(), id, "isolated input-rerun cleanup") })
	observe := func(status string) *conductor.WorkflowExecution {
		t.Helper()
		for {
			e, err := api.GetWorkflow(ctx, id)
			if err != nil {
				t.Fatal("isolated native execution unavailable")
			}
			if e.Status == status && len(e.Tasks) > 0 {
				return e
			}
			select {
			case <-ctx.Done():
				t.Fatal("isolated native observation timed out")
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	before := observe("RUNNING")
	oldTask := before.Tasks[0].TaskID
	if err := api.UpdateTask(ctx, conductor.TaskResult{TaskID: oldTask, WorkflowInstanceID: id, Status: "FAILED_WITH_TERMINAL_ERROR", WorkerID: "isolated-fixture", ReasonForIncompletion: "fixture failure"}); err != nil {
		t.Fatal("isolated failure injection rejected")
	}
	observe("FAILED")
	jobs, steps := repo.NewJobRepo(db), repo.NewStepRunRepo(db)
	j := &domain.Job{ID: name, WorkflowID: name, WorkflowVersion: 1, Scope: "e2e", SubmittedBy: "owner", Status: domain.JobSucceeded}
	sr := &domain.StepRun{ID: name + "_step", JobID: name, StepKey: "fixture", TemplateID: "template", TemplateVersion: 1, Status: domain.SRSucceeded}
	if jobs.Save(ctx, j, []*domain.StepRun{sr}) != nil || jobs.UpdateConductorExecution(ctx, name, id, strings.Repeat("a", 64)) != nil {
		t.Fatal("isolated business projection seed failed")
	}
	ir, _ := json.Marshal(executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: name, WorkflowVersion: 1})
	comp := retryCommitCompilation{item: &wfdomain.Compilation{DefinitionHash: strings.Repeat("a", 64), IRJSON: ir}}
	upstream, err := url.Parse(address)
	if err != nil {
		t.Fatal("invalid upstream")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var posts atomic.Int32
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.Request.Method == http.MethodPost && strings.HasSuffix(resp.Request.URL.Path, "/rerun") {
			posts.Add(1)
			if resp.StatusCode < 300 {
				resp.Body.Close()
				return errors.New("fixture response lost after real commit")
			}
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) { w.WriteHeader(http.StatusBadGateway) }
	srv := httptest.NewServer(proxy)
	defer srv.Close()
	engine := &conductor.ExecutionEngine{Client: conductor.NewClient(srv.URL)}
	svc := service.NewService(jobs, steps, dal.NewTxRunner(db), nil, nil, nil, nil, comp, engine)
	svc.ConfigureReviews(jobs)
	got, err := svc.Retry(ctx, name, "change input", &domain.JobInput{})
	if err != nil || got.Status != domain.JobRunning || got.RetryCount != 1 || posts.Load() != 1 {
		t.Fatal("lost rerun acknowledgement was not recovered exactly once", err)
	}
	after := observe("RUNNING")
	for _, task := range after.Tasks {
		if task.TaskID == oldTask {
			t.Fatal("native whole rerun retained an old task")
		}
	}
	if _, err := svc.Retry(ctx, name, "repeat input override", &domain.JobInput{}); err == nil || posts.Load() != 1 {
		t.Fatal("running instance received a second rerun")
	}
	stored, err := jobs.Get(ctx, name)
	if err != nil || stored.RetryCount != 1 || len(stored.RetryHistory) != 1 {
		t.Fatal("rerun audit was duplicated or lost")
	}
}

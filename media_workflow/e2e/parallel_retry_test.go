//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
)

// A contract hazard probe, not a successful business parallel-retry gate.
// Conductor truncates a completed sibling by sequence rather than DAG edges.
func TestConductorParallelRerunTruncatesCompletedSibling(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	name := "parallel_rerun_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	api := conductor.NewClient(address)
	definition := compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{
		{Name: "fork", TaskReferenceName: "fork", Type: "FORK_JOIN", ForkTasks: [][]compiler.Task{
			{{Name: "a", TaskReferenceName: "a", Type: "HUMAN"}},
			{{Name: "b", TaskReferenceName: "b", Type: "HUMAN"}},
		}},
		{Name: "join", TaskReferenceName: "join", Type: "JOIN", JoinOn: []string{"a", "b"}},
	}}
	if err := api.RegisterWorkflow(ctx, definition); err != nil {
		t.Fatal("isolated definition registration failed")
	}
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, address+"/api/metadata/workflow/"+name+"/1", nil)
		r, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Error("isolated definition cleanup failed")
			return
		}
		r.Body.Close()
		if r.StatusCode >= 300 {
			t.Error("isolated definition cleanup rejected")
		}
	})
	id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: name, Version: 1, Input: map[string]any{}})
	if err != nil {
		t.Fatal("isolated execution start failed")
	}
	t.Cleanup(func() { _ = api.TerminateWorkflow(context.Background(), id, "isolated parallel rerun cleanup") })
	wait := func(predicate func(*conductor.WorkflowExecution) bool) *conductor.WorkflowExecution {
		t.Helper()
		for {
			execution, err := api.GetWorkflow(ctx, id)
			if err != nil {
				t.Fatal("isolated execution unavailable")
			}
			if predicate(execution) {
				return execution
			}
			select {
			case <-ctx.Done():
				t.Fatal("isolated execution did not converge")
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	var a, b conductor.Task
	wait(func(e *conductor.WorkflowExecution) bool {
		for _, task := range e.Tasks {
			if task.ReferenceTaskName == "a" {
				a = task
			}
			if task.ReferenceTaskName == "b" {
				b = task
			}
		}
		return a.Status == "IN_PROGRESS" && b.Status == "IN_PROGRESS"
	})
	if a.Sequence == nil || b.Sequence == nil || *b.Sequence <= *a.Sequence {
		t.Fatal("probe requires sibling after target in authoritative sequence")
	}
	if err := api.CompleteTaskByReference(ctx, id, "b", map[string]any{"result": "retained-sibling"}); err != nil {
		t.Fatal("sibling completion failed")
	}
	if err := api.UpdateTask(ctx, conductor.TaskResult{TaskID: a.TaskID, WorkflowInstanceID: id, Status: "FAILED_WITH_TERMINAL_ERROR", WorkerID: "isolated-parallel-probe", ReasonForIncompletion: "isolated failure"}); err != nil {
		t.Fatal("target failure failed")
	}
	wait(func(e *conductor.WorkflowExecution) bool { return e.Status == "FAILED" })
	if err := api.RerunFromTask(ctx, id, a.TaskID); err != nil {
		t.Fatal("isolated rerun failed")
	}
	execution, err := api.GetWorkflow(ctx, id)
	if err != nil {
		t.Fatal("rerun execution unavailable")
	}
	for _, task := range execution.Tasks {
		if task.TaskID == b.TaskID {
			t.Fatal("upstream rerun behavior changed: completed sibling no longer truncated; revisit isolation gate")
		}
	}
	t.Log("rerun removed the completed unrelated sibling; business retry must protect it")
}

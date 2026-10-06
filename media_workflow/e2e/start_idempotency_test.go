//go:build e2e

package e2e

import (
	"context"
	"io"
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
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
)

func TestConductorStartCommittedResponseLossRecovery(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name := "start_loss_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	api := conductor.NewClient(address)
	definition := compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: "review", TaskReferenceName: "review", Type: "HUMAN"}}}
	if err := api.RegisterWorkflow(ctx, definition); err != nil {
		t.Fatal("isolated start definition registration failed")
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
	upstream, err := url.Parse(address)
	if err != nil {
		t.Fatal("invalid configured upstream")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var posts atomic.Int32
	var committed atomic.Value
	proxy.ModifyResponse = func(r *http.Response) error {
		if r.Request.Method != http.MethodPost || r.Request.URL.Path != "/api/workflow" {
			return nil
		}
		posts.Add(1)
		if r.StatusCode >= 300 {
			return nil
		}
		body, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			return err
		}
		id := strings.Trim(strings.TrimSpace(string(body)), `"`)
		committed.Store(id)
		r.StatusCode = 503
		r.Body = io.NopCloser(strings.NewReader("injected response loss"))
		r.ContentLength = int64(len("injected response loss"))
		r.Header.Del("Content-Length")
		return nil
	}
	server := httptest.NewServer(proxy)
	defer server.Close()
	t.Cleanup(func() {
		if value := committed.Load(); value != nil {
			_ = api.TerminateWorkflow(context.Background(), value.(string), "isolated response loss cleanup")
		}
	})
	engine := &conductor.ExecutionEngine{Client: conductor.NewClient(server.URL)}
	id, err := engine.Start(ctx, name, 1, name, map[string]any{"job_id": name}, nil)
	if err != nil || committed.Load() == nil || id != committed.Load().(string) || posts.Load() != 1 {
		t.Fatal("real committed Start was not recovered without resubmission")
	}
	t.Log("recovered the real committed execution through correlation with exactly one Start request")
}

// This gate must pass before replaying ambiguous Start commands automatically.
// A correlation lookup alone is not a uniqueness guarantee.
func TestConductorStartIdempotencyContract(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name := "start_idempotency_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	api := conductor.NewClient(address)
	definition := compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: "review", TaskReferenceName: "review", Type: "HUMAN"}}}
	if err := api.RegisterWorkflow(ctx, definition); err != nil {
		t.Fatal("isolated start definition registration failed")
	}
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, address+"/api/metadata/workflow/"+name+"/1", nil)
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Error("isolated start definition cleanup failed")
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			t.Error("isolated start definition cleanup rejected")
		}
	})
	request := conductor.StartWorkflowRequest{Name: name, Version: 1, CorrelationID: name,
		IdempotencyKey: name, IdempotencyStrategy: "RETURN_EXISTING", Input: map[string]any{"job_id": name}}
	start := func() string {
		t.Helper()
		id, err := api.StartWorkflow(ctx, request)
		if err != nil {
			t.Fatal("isolated start request failed")
		}
		t.Cleanup(func() { _ = api.TerminateWorkflow(context.Background(), id, "isolated start contract cleanup") })
		return id
	}
	first, second := start(), start()
	if first != second {
		t.Fatal("Conductor created distinct executions for the same RETURN_EXISTING key; automatic Start replay is unsafe")
	}
	if err := api.CompleteTaskByReference(ctx, first, "review", map[string]any{"decision": "accept"}); err != nil {
		t.Fatal("isolated task completion failed")
	}
	if third := start(); third != first {
		t.Fatal("terminal execution was not reused by RETURN_EXISTING")
	}
}

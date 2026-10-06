//go:build e2e

package e2e

import (
	"context"
	"errors"
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

// Only a unique technical HUMAN definition is created. No workflow instance,
// task definition, user queue, model or production compilation is touched.
func TestConductorRegistrationFrozenDefinition(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name := "registration_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	definition := compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: "human", TaskReferenceName: "review", Type: "HUMAN"}}}
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, address+"/api/metadata/workflow/"+name+"/1", nil)
		response, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Error("isolated definition cleanup failed")
			return
		}
		response.Body.Close()
		if response.StatusCode >= 300 && response.StatusCode != 404 {
			t.Error("isolated definition cleanup rejected")
		}
	})
	upstream, err := url.Parse(address)
	if err != nil {
		t.Fatal("invalid configured upstream")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var writes atomic.Int32
	proxy.ModifyResponse = func(response *http.Response) error {
		if response.Request.Method != http.MethodPut || response.Request.URL.Path != "/api/metadata/workflow" {
			return nil
		}
		writes.Add(1)
		if response.StatusCode >= 300 {
			return nil
		}
		response.Body.Close()
		response.StatusCode = 503
		response.Body = io.NopCloser(strings.NewReader("injected committed response loss"))
		response.ContentLength = -1
		response.Header.Del("Content-Length")
		return nil
	}
	server := httptest.NewServer(proxy)
	defer server.Close()
	api := conductor.NewClient(server.URL)
	if err := api.EnsureWorkflow(ctx, definition); err != nil {
		t.Fatal("committed registration did not recover by exact metadata lookup")
	}
	if writes.Load() != 1 {
		t.Fatal("registration repeated an uncertain write")
	}
	if err := api.EnsureWorkflow(ctx, definition); err != nil || writes.Load() != 1 {
		t.Fatal("matching metadata replay performed a write")
	}
	changed := definition
	changed.Description = "conflicting frozen compilation"
	if err := api.EnsureWorkflow(ctx, changed); !errors.Is(err, conductor.ErrWorkflowDefinitionConflict) || writes.Load() != 1 {
		t.Fatal("conflicting definition was overwritten")
	}
	if err := api.EnsureWorkflow(ctx, definition); err != nil {
		t.Fatal("conflict rejection changed the original definition")
	}
}

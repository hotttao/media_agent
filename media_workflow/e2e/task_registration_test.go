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
	"media_agent/media_workflow/biz/workflow/conductor"
)

// Real Conductor with a unique technical task definition. No Workflow instance,
// Worker, existing task metadata, model or application database is touched.
func TestConductorTaskDefinitionFrozenRegistration(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name := "task_registration_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, address+"/api/metadata/taskdefs/"+name, nil)
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Error("isolated task definition cleanup failed")
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 && resp.StatusCode != 404 {
			t.Error("isolated task definition cleanup rejected")
		}
	})
	upstream, err := url.Parse(address)
	if err != nil {
		t.Fatal("invalid configured upstream")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var posts, puts atomic.Int32
	proxy.ModifyResponse = func(response *http.Response) error {
		if response.Request.Method == http.MethodPut {
			puts.Add(1)
		}
		if response.Request.Method != http.MethodPost || response.Request.URL.Path != "/api/metadata/taskdefs" {
			return nil
		}
		posts.Add(1)
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
	definition := conductor.TaskDefinition{Name: name, RetryCount: 0, TimeoutSeconds: 120, ResponseTimeoutSeconds: 60, TimeoutPolicy: "TIME_OUT_WF"}
	if api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{definition}) != nil || posts.Load() != 1 || puts.Load() != 0 {
		t.Fatal("real committed task registration did not recover by lookup")
	}
	if api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{definition}) != nil || posts.Load() != 1 {
		t.Fatal("exact real task definition replay wrote metadata")
	}
	changed := definition
	changed.RetryCount = 9
	if err := api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{changed}); !errors.Is(err, conductor.ErrTaskDefinitionConflict) || posts.Load() != 1 || puts.Load() != 0 {
		t.Fatal("real conflicting task metadata was overwritten")
	}
	if api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{definition}) != nil {
		t.Fatal("original task metadata lost after conflict")
	}
}

// Explicit read-only compatibility gate for the six task names currently used
// by the published four-node sample. A conflicting name must not be repaired.
func TestConductorPublishedTaskDefinitionsCompatibility(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" || os.Getenv("MEDIA_E2E_EXISTING_TASK_DEFINITIONS") != "1" {
		t.Skip("explicit read-only existing task compatibility gate")
	}
	upstream, err := url.Parse(address)
	if err != nil {
		t.Fatal("invalid upstream")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
			w.WriteHeader(503)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer server.Close()
	definitions := []conductor.TaskDefinition{
		{Name: "media_render_template_v1", RetryCount: 2, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutSeconds: 60, ResponseTimeoutSeconds: 30, TimeoutPolicy: "TIME_OUT_WF"},
		{Name: "media_finalize_step_v1", RetryCount: 2, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutSeconds: 60, ResponseTimeoutSeconds: 30, TimeoutPolicy: "TIME_OUT_WF"},
	}
	for _, name := range []string{"media_image_model_v1", "media_image_style_v1", "media_image_first_frame_v1", "media_video_generate_v1"} {
		definitions = append(definitions, conductor.TaskDefinition{Name: name, RetryCount: 3, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutSeconds: 600, ResponseTimeoutSeconds: 600, TimeoutPolicy: "TIME_OUT_WF"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if conductor.NewClient(server.URL).RegisterTaskDefinitions(ctx, definitions) != nil || writes.Load() != 0 {
		t.Fatal("published sample task contracts are incompatible; no metadata repaired")
	}
}

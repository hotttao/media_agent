package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIWorkflowFailureExitsNonzeroWithoutLeakingResponse(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "media_worker")
	build := exec.Command("go", "build", "-o", binary, "../cmd/media_worker")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build worker: %s %v", out, err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/worker/v1/registrations" {
			t.Error("unexpected call before registration")
		}
		http.Error(w, "private-response-sentinel", http.StatusUnauthorized)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-gateway-url", server.URL, "-token", "private-token-sentinel", "-worker-id", "test-worker", "-profile", "media_ai_fixture", "-capabilities", "media.video.generate@1", "-state-dir", filepath.Join(t.TempDir(), "private"))
	// Isolate fixture profile configuration from the developer's shell.
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "MEDIA_WORKER_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil || err == nil || !strings.Contains(string(out), "register worker:") || strings.Contains(string(out), "private-response-sentinel") || strings.Contains(string(out), "private-token-sentinel") {
		t.Fatalf("workflow failure did not exit safely: %s %v", out, err)
	}
}

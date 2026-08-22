package e2e

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestCodexRuntimeAuthAndStartup verifies the image contract without reading
// the credential contents: the entrypoint must install auth.json with 0600
// permissions and the ACP executable must be available to the agent.
func TestCodexRuntimeAuthAndStartup(t *testing.T) {
	container, addr := os.Getenv("MEDIA_AGENTS_CODEX_CONTAINER"), os.Getenv("MEDIA_AGENTS_ADDR")
	if container == "" || addr == "" {
		t.Skip("MEDIA_AGENTS_CODEX_CONTAINER and MEDIA_AGENTS_ADDR are required")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	waitAgentHealthy(t, client, addr)
	output, err := exec.Command("docker", "exec", container, "sh", "-lc",
		"test -f /workspace/.home/.codex/auth.json && "+
			"test \"$(stat -c %a /workspace/.home/.codex/auth.json)\" = 600 && "+
			"command -v codex-acp").CombinedOutput()
	if err != nil {
		t.Fatalf("Codex image contract failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	if strings.Contains(string(output), "auth.json") {
		t.Fatalf("auth path unexpectedly printed by probe: %s", output)
	}
	if len(strings.TrimSpace(string(output))) == 0 {
		t.Fatal(fmt.Errorf("codex-acp executable path was not reported"))
	}
}

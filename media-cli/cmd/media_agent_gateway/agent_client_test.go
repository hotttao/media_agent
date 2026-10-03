package media_agent_gateway

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentSecretFilePrivateAndNoOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	if err := writeAgentSecret(path, "one-time"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("Secret file permissions")
	}
	if err := writeAgentSecret(path, "replacement"); err == nil {
		t.Fatal("existing Secret file overwritten")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "MEDIA_API_TOKEN=one-time\n" {
		t.Fatal("Secret content changed")
	}
}

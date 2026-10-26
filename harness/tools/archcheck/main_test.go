package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual CLI in a multi-module workspace. A path normalization
// regression must not silently accept domain -> client/repository imports.
func TestArchitectureCLIInWorkspace(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "archcheck")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", out, err)
	}
	for _, tc := range []struct {
		name, layer string
		rejected    bool
	}{
		{"domain-to-client", "biz/client/remote", true},
		{"domain-to-repository", "biz/dal/repo/results", true},
		{"domain-to-shared", "biz/shared/value", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				"go.work":                           "go 1.26.3\nuse ./worker\n",
				"worker/go.mod":                     "module arch.example/worker\ngo 1.26.3\n",
				"worker/biz/domain/task/task.go":    "package task\nimport _ \"arch.example/worker/" + tc.layer + "\"\n",
				"worker/" + tc.layer + "/target.go": "package target\n",
			}
			for name, data := range files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0644); err != nil {
					t.Fatal(err)
				}
			}
			git := exec.Command("git", "init", "-q", root)
			if out, err := git.CombinedOutput(); err != nil {
				t.Fatalf("git: %s %v", out, err)
			}
			cmd := exec.Command(binary)
			cmd.Dir = filepath.Join(root, "worker")
			cmd.Env = append(os.Environ(), "GOWORK="+filepath.Join(root, "go.work"))
			out, err := cmd.CombinedOutput()
			if tc.rejected {
				if err == nil || !strings.Contains(string(out), "domain-must-not-import-outer-layers") {
					t.Fatalf("expected dependency rejection: %s %v", out, err)
				}
			} else if err != nil || !strings.Contains(string(out), "architecture check passed") {
				t.Fatalf("allowed dependency rejected: %s %v", out, err)
			}
		})
	}
}

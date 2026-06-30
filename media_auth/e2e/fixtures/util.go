//go:build e2e

// util.go：e2e 辅助（resolveMigrationsDir 等）。
package fixtures

import (
	"os"
	"path/filepath"
	"testing"
)

// resolveMigrationsDir 从测试 cwd（media_auth/e2e 或 e2e/fixtures）向上找 media_auth/migrations。
func resolveMigrationsDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		candidate := filepath.Join(dir, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("migrations dir not found from %s", wd)
		}
		dir = parent
	}
}

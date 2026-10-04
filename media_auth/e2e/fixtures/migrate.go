//go:build e2e

// migrate.go：e2e 迁移 fixture——对隔离 database 跑 migrations/0001_init。
package fixtures

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_auth/biz/dal"
)

// Migrate 对给定 database 执行 media_auth 全部迁移。
func Migrate(t *testing.T, db *Database) {
	t.Helper()
	cfg := &configpb.DatabaseConfig{
		Enabled:        true,
		Driver:         "postgres",
		Dsn:            db.DSN,
		MigrateOnStart: true,
	}
	// migrations 目录在项目根（media_auth/migrations）；dal.Migrate 用相对路径 "migrations"，
	// 但 e2e 的 cwd 是 media_auth/e2e，故用 resolveMigrationsDir 找上级。
	dir := resolveMigrationsDir(t)
	if err := dal.Migrate(dir, cfg); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// MigrateToVersion is only for historical migration scenarios. Production
// continues to apply every migration; ordinary fixtures also use Migrate.
func MigrateToVersion(t *testing.T, db *Database, version int) {
	t.Helper()
	if version < 1 {
		t.Fatal("invalid historical schema version")
	}
	dir := resolveMigrationsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal("historical migrations unavailable")
	}
	selected := t.TempDir()
	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		n, err := strconv.Atoi(prefix)
		if !ok || err != nil || n > version {
			continue
		}
		found = found || n == version
		contents, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil || os.WriteFile(filepath.Join(selected, entry.Name()), contents, 0600) != nil {
			t.Fatal("historical migration fixture preparation failed")
		}
	}
	if !found {
		t.Fatal("historical migration version unavailable")
	}
	if err := dal.Migrate(selected, &configpb.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: db.DSN, MigrateOnStart: true}); err != nil {
		t.Fatal("historical schema migration failed")
	}
}

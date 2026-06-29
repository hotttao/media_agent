// migrate.go：迁移 fixture——把 dal.Migrate 封装成测试可用的 helper。
//
// 迁移 SQL 不再 embed，运行时从磁盘读（与 main 一致）。e2e 测试的 cwd 是 e2e/ 目录，
// migrations 在上级 ../migrations，用 resolveMigrationsDir 解析为绝对路径，避免依赖 cwd。
package fixtures

import (
	"os"
	"path/filepath"
	"testing"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_example/biz/dal"
)

// Migrate 对给定 database 执行迁移（用独立 *sql.DB，不碰 gorm 连接）。
// cfg.migrate_on_start=true + db.dsn 触发迁移；已最新幂等。
func Migrate(t *testing.T, db *Database) {
	t.Helper()
	dir := resolveMigrationsDir(t)
	cfg := &configpb.DatabaseConfig{Enabled: true, MigrateOnStart: true, Dsn: db.DSN}
	if err := dal.Migrate(dir, cfg); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// resolveMigrationsDir 解析 migrations 目录的绝对路径。
// 从测试 cwd（e2e/ 或 e2e/fixtures）向上找 migrations/，兼容不同测试包的 cwd。
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

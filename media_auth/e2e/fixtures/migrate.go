//go:build e2e

// migrate.go：e2e 迁移 fixture——对隔离 database 跑 migrations/0001_init。
package fixtures

import (
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

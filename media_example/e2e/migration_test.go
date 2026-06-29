//go:build e2e

// migration_test.go：数据库 migration 正确性 + 幂等性 e2e。
package e2e

import (
	"testing"

	"media_agent/media_example/e2e/fixtures"
)

// TestMigrate_CreatesTables：迁移后业务表存在。
func TestMigrate_CreatesTables(t *testing.T) {
	db := fixtures.NewDatabase(t)
	fixtures.Migrate(t, db)

	for _, tbl := range []string{"greetings", "likes", "post_counters"} {
		if !fixtures.TableExists(t, db.Gorm, tbl) {
			t.Errorf("after migrate: table %q should exist", tbl)
		}
	}
}

// TestMigrate_Idempotent：二次迁移无副作用（ErrNoChange 视为成功）。
func TestMigrate_Idempotent(t *testing.T) {
	db := fixtures.NewDatabase(t)
	fixtures.Migrate(t, db)
	// 二次迁移应幂等成功。
	fixtures.Migrate(t, db)
}

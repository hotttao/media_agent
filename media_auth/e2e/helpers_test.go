//go:build e2e

// helpers_test.go：e2e 共享测试辅助（build tag e2e）。
//
// 每个 repo 集成测试经 newDB 取一个隔离 database（fixtures.NewDatabase + Migrate），
// 再 seed 所需 user。隔离粒度 = 每测试独立 database，无需 resetTables。
package e2e

import (
	"testing"

	"media_agent/media_auth/e2e/fixtures"
)

// newDB 返回一个已迁移的隔离 *gorm.DB（每测试独立 database）。
func newDB(t *testing.T) *fixtures.Database {
	t.Helper()
	db := fixtures.NewDatabase(t)
	fixtures.Migrate(t, db)
	return db
}

// seedUser 插入一个测试 user。
func seedUser(t *testing.T, db *fixtures.Database, id, username string) {
	t.Helper()
	if err := db.Gorm.Exec(
		`INSERT INTO media_auth.users (id, username, display_name, role) VALUES (?, ?, ?, 'user')`,
		id, username, username,
	).Error; err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

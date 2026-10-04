//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"media_agent/media_auth/biz/dal"
	authrepo "media_agent/media_auth/biz/dal/repo/auth"
	platformrepo "media_agent/media_auth/biz/dal/repo/platform_token"
	"media_agent/media_auth/biz/workflow"
	"media_agent/media_auth/e2e/fixtures"
)

func retirementSQL(t *testing.T, suffix string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "migrations", "0017_retire_service_tokens."+suffix+".sql"))
	if err != nil {
		t.Fatal("retirement migration unavailable")
	}
	// Execute the file body inside our own transaction, so expected failures
	// roll back on the same connection instead of leaving a pooled failed TX.
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(string(contents)), "BEGIN;"), "COMMIT;")
}

func runRetirementSQL(t *testing.T, db *fixtures.Database, statement string) error {
	t.Helper()
	pool, err := db.Gorm.DB()
	if err != nil {
		t.Fatal("isolated SQL connection unavailable")
	}
	tx, err := pool.BeginTx(t.Context(), &sql.TxOptions{})
	if err != nil {
		t.Fatal("isolated SQL transaction unavailable")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(t.Context(), statement); err != nil {
		return err
	}
	return tx.Commit()
}

func archiveFingerprint(t *testing.T, db *fixtures.Database) string {
	t.Helper()
	var checksum string
	if err := db.Gorm.Raw(`SELECT COALESCE(md5(string_agg(row_to_json(s)::text, E'\n' ORDER BY id)),'empty') FROM media_auth.service_tokens s`).Scan(&checksum).Error; err != nil {
		t.Fatal("isolated history verification failed")
	}
	return checksum // Only synthetic fixture data; never printed or persisted.
}

func TestRetiredServiceTokenArchivePreservesHistoryAndRejectsWrites(t *testing.T) {
	db := newDBAtVersion(t, 16)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "operator", "operator")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.service_tokens(id,name,token_hash,created_by,created_at,revoked_at,expires_at,is_platform) VALUES ('ordinary','history','fixture-hash','owner','2026-01-01','2026-01-02','2026-01-03',false)`).Error; err != nil {
		t.Fatal("ordinary history fixture unavailable")
	}
	before := archiveFingerprint(t, db)
	if err := runRetirementSQL(t, db, retirementSQL(t, "up")); err != nil {
		t.Fatal("read-only retirement failed")
	}
	if archiveFingerprint(t, db) != before {
		t.Fatal("retirement changed credential history")
	}
	for _, statement := range []string{
		`INSERT INTO media_auth.service_tokens(id,name,token_hash,is_platform) VALUES ('new','forbidden','fixture-hash',false)`,
		`UPDATE media_auth.service_tokens SET name='changed' WHERE id='ordinary'`,
		`UPDATE media_auth.service_tokens SET name='changed' WHERE id='absent'`,
		`DELETE FROM media_auth.service_tokens WHERE id='ordinary'`,
		`DELETE FROM media_auth.service_tokens WHERE id='absent'`,
		`TRUNCATE media_auth.service_tokens`,
		`TRUNCATE media_auth.service_tokens CASCADE`,
		`SET LOCAL session_replication_role=replica; DELETE FROM media_auth.service_tokens`,
	} {
		err := runRetirementSQL(t, db, statement)
		if err == nil || !strings.Contains(err.Error(), "retired credential archive is read-only") || archiveFingerprint(t, db) != before {
			t.Fatal("archive write admitted or failed for an unrelated reason")
		}
	}
	if err := runRetirementSQL(t, db, retirementSQL(t, "down")); err == nil || !strings.Contains(err.Error(), "cannot be automatically rolled back") {
		t.Fatal("retirement allowed unsafe downgrade")
	}
	ring := platformTestRing(t)
	flow := workflow.NewPlatformTokenMigration(dal.NewTxRunner(db.Gorm), platformrepo.NewPlatformMigrationSource(db.Gorm), platformrepo.NewRepo(db.Gorm), ring)
	for _, apply := range []bool{false, true} {
		if result, err := flow.Run(t.Context(), apply); err != nil || result.Verified != 0 || result.Committed != apply {
			t.Fatal("completed migration cannot be safely rerun after archive")
		}
	}
	// Current lifecycle cleanup must not update this retired table anymore.
	custody := platformCore(t, db, nil)
	if err := custody.EnsurePlatformToken(t.Context(), "owner"); err != nil {
		t.Fatal("independent custody fixture unavailable")
	}
	users, outbox := authrepo.NewRepo(db.Gorm), authrepo.NewAccessRevocationOutbox(db.Gorm)
	revocation := workflow.NewUserSoftDelete(users, outbox, custody, &localCredentialCleanup{}, &localCredentialCleanup{}, dal.NewTxRunner(db.Gorm))
	if err := revocation.UpdateStatus(t.Context(), "operator", "owner", "disabled"); err != nil || revocation.ProcessPending(context.Background()) != nil {
		t.Fatal("current User lifecycle still depends on retired writes")
	}
	var pending int64
	if db.Gorm.Table("media_auth.user_access_outbox").Where("delivered_at IS NULL").Count(&pending).Error != nil || pending != 0 || archiveFingerprint(t, db) != before {
		t.Fatal("revocation did not converge independently of archive")
	}
}

func TestRetiredServiceTokenArchiveRequiresCompletePlatformTransfer(t *testing.T) {
	db := newDBAtVersion(t, 16)
	seedUser(t, db, "owner", "owner")
	seedLegacyPlatform(t, db, 0, true)
	before := archiveFingerprint(t, db)
	err := runRetirementSQL(t, db, retirementSQL(t, "up"))
	if err == nil || !strings.Contains(err.Error(), "platform custody migration is required") || archiveFingerprint(t, db) != before {
		t.Fatal("archive ignored unmigrated Platform history")
	}
	ring := platformTestRing(t)
	flow := workflow.NewPlatformTokenMigration(dal.NewTxRunner(db.Gorm), platformrepo.NewPlatformMigrationSource(db.Gorm), platformrepo.NewRepo(db.Gorm), ring)
	if result, err := flow.Run(t.Context(), true); err != nil || result.Verified != 1 {
		t.Fatal("failed archive left partial guards or broke Platform transfer")
	}
	if runRetirementSQL(t, db, retirementSQL(t, "up")) != nil {
		t.Fatal("verified transfer cannot advance to read-only archive")
	}
}

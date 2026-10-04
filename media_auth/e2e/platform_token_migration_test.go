//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"media_agent/media_auth/biz/dal"
	platformrepo "media_agent/media_auth/biz/dal/repo/platform_token"
	legacyrepo "media_agent/media_auth/biz/dal/repo/service_token"
	platform "media_agent/media_auth/biz/domain/platform_token"
	legacy "media_agent/media_auth/biz/domain/service_token"
	"media_agent/media_auth/biz/shared/platformcipher"
	"media_agent/media_auth/biz/workflow"
	"media_agent/media_auth/e2e/fixtures"
)

func seedLegacyPlatform(t *testing.T, db *fixtures.Database, n int, revoked bool) legacy.LegacyPlatformRecord {
	t.Helper()
	timepoint := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	row := legacy.LegacyPlatformRecord{ID: fmt.Sprintf("legacy-%04d", n), UserID: "owner", Secret: fmt.Sprintf("tos_%064x", n+1), CreatedAt: timepoint}
	hash := sha256.Sum256([]byte(row.Secret))
	row.Hash = hex.EncodeToString(hash[:])
	if revoked {
		at := timepoint.Add(time.Hour)
		row.RevokedAt = &at
	}
	if n%2 == 0 {
		at := timepoint.Add(2 * time.Hour)
		row.ExpiresAt = &at
	}
	if err := db.Gorm.Exec(`INSERT INTO media_auth.service_tokens(id,name,token_hash,token,created_by,created_at,expires_at,revoked_at,is_platform) VALUES (?, '__platform__', ?, ?, ?, ?, ?, ?, true)`, row.ID, row.Hash, row.Secret, row.UserID, row.CreatedAt, row.ExpiresAt, row.RevokedAt).Error; err != nil {
		t.Fatal("legacy fixture creation failed")
	}
	return row
}

func saveMatchingPlatform(t *testing.T, repo *platformrepo.Repo, ring *platformcipher.Keyring, old legacy.LegacyPlatformRecord) {
	t.Helper()
	env, err := ring.Seal(old.ID, old.UserID, old.Secret)
	if err != nil {
		t.Fatal("fixture encryption failed")
	}
	if err := repo.Create(t.Context(), platform.Token{ID: old.ID, UserID: old.UserID, TokenHash: env.TokenHash, Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyVersion: env.KeyVersion, CreatedAt: old.CreatedAt, ExpiresAt: old.ExpiresAt, RevokedAt: old.RevokedAt}); err != nil {
		t.Fatal("target fixture creation failed")
	}
}

func migrationCounts(t *testing.T, db *fixtures.Database, wantOld, wantTarget int) {
	t.Helper()
	var old, target int64
	if db.Gorm.Table("media_auth.service_tokens").Where("is_platform = true").Count(&old).Error != nil || db.Gorm.Table("media_auth.platform_tokens").Count(&target).Error != nil || old != int64(wantOld) || target != int64(wantTarget) {
		t.Fatal("unexpected transfer counts or partial mutation")
	}
}

func TestPlatformTokenMigrationDryRunPagingConcurrentApplyAndHistory(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	ring := platformTestRing(t)
	target := platformrepo.NewRepo(db.Gorm)
	source := legacyrepo.NewPlatformMigrationSource(db.Gorm)
	flow := workflow.NewPlatformTokenMigration(dal.NewTxRunner(db.Gorm), source, target, ring)
	rows := make([]legacy.LegacyPlatformRecord, 105)
	for i := range rows {
		rows[i] = seedLegacyPlatform(t, db, i, i != len(rows)-1)
	}
	saveMatchingPlatform(t, target, ring, rows[0])
	if err := db.Gorm.Exec(`INSERT INTO media_auth.service_tokens(id,name,token_hash,created_by,is_platform) VALUES ('generic','keep','generic-hash','owner',false)`).Error; err != nil {
		t.Fatal(err)
	}
	result, err := flow.Run(t.Context(), false)
	if err != nil || result.Committed || result.Verified != 105 || result.Created != 104 || result.Reused != 1 {
		t.Fatal("dry run did not verify complete source")
	}
	migrationCounts(t, db, 105, 1)
	var wg sync.WaitGroup
	results := make(chan workflow.PlatformMigrationResult, 2)
	errorsCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, err := flow.Run(t.Context(), true); results <- r; errorsCh <- err }()
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal("concurrent transfer failed")
		}
	}
	verified := 0
	for r := range results {
		if !r.Committed {
			t.Fatal("apply did not commit")
		}
		verified += r.Verified
	}
	if verified != 105 {
		t.Fatal("concurrent transfer duplicated or omitted source")
	}
	migrationCounts(t, db, 0, 105)
	for _, old := range rows {
		saved, err := target.GetIncludingRevoked(t.Context(), old.ID)
		if err != nil || saved.UserID != old.UserID || !saved.CreatedAt.Equal(old.CreatedAt) || !migrationTimeEqual(old.ExpiresAt, saved.ExpiresAt) || !migrationTimeEqual(old.RevokedAt, saved.RevokedAt) {
			t.Fatal("history or ownership changed")
		}
		secret, err := ring.Open(saved.ID, saved.UserID, platformcipher.Envelope{KeyVersion: saved.KeyVersion, TokenHash: saved.TokenHash, Ciphertext: saved.Ciphertext, Nonce: saved.Nonce})
		if err != nil || secret != old.Secret {
			t.Fatal("transferred credential changed")
		}
	}
	var generic int64
	if db.Gorm.Table("media_auth.service_tokens").Where("id='generic' AND token IS NULL AND token_hash='generic-hash' AND revoked_at IS NULL AND is_platform=false").Count(&generic).Error != nil || generic != 1 {
		t.Fatal("ordinary ServiceToken was changed")
	}
	if result, err := flow.Run(t.Context(), true); err != nil || result.Verified != 0 || !result.Committed {
		t.Fatal("completed transfer was not idempotent")
	}
	if err := source.LockPlatformMigration(t.Context()); err == nil {
		t.Fatal("lock outside transaction was accepted")
	}
	if err := source.DeleteMigratedPlatform(t.Context(), "generic"); err == nil {
		t.Fatal("delete outside transaction was accepted")
	}
}

func migrationTimeEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func TestPlatformTokenMigrationBadSourceOrConflictingTargetRollsBackAll(t *testing.T) {
	for _, fault := range []string{"hash", "missing-owner", "empty-id", "different-history", "unreadable-target"} {
		t.Run(fault, func(t *testing.T) {
			db := newDB(t)
			seedUser(t, db, "owner", "owner")
			ring := platformTestRing(t)
			target := platformrepo.NewRepo(db.Gorm)
			seedLegacyPlatform(t, db, 0, true)
			bad := seedLegacyPlatform(t, db, 1, false)
			wantTarget := 0
			var mutationErr error
			switch fault {
			case "hash":
				mutationErr = db.Gorm.Exec("UPDATE media_auth.service_tokens SET token_hash=? WHERE id=?", fmt.Sprintf("%064x", 0), bad.ID).Error
			case "missing-owner":
				mutationErr = db.Gorm.Exec("UPDATE media_auth.service_tokens SET created_by=NULL WHERE id=?", bad.ID).Error
			case "empty-id":
				mutationErr = db.Gorm.Exec("UPDATE media_auth.service_tokens SET id='' WHERE id=?", bad.ID).Error
			case "different-history":
				saveMatchingPlatform(t, target, ring, bad)
				wantTarget = 1
				mutationErr = db.Gorm.Exec("UPDATE media_auth.platform_tokens SET created_at=created_at+interval '1 second' WHERE id=?", bad.ID).Error
			case "unreadable-target":
				saveMatchingPlatform(t, target, ring, bad)
				wantTarget = 1
				mutationErr = db.Gorm.Exec("UPDATE media_auth.platform_tokens SET key_version='unavailable' WHERE id=?", bad.ID).Error
			}
			if mutationErr != nil {
				t.Fatal("fault fixture mutation failed")
			}
			flow := workflow.NewPlatformTokenMigration(dal.NewTxRunner(db.Gorm), legacyrepo.NewPlatformMigrationSource(db.Gorm), target, ring)
			if result, err := flow.Run(t.Context(), true); !errors.Is(err, workflow.ErrPlatformMigration) || result.Verified != 0 || result.Committed {
				t.Fatal("fault did not fail closed")
			}
			migrationCounts(t, db, 2, wantTarget)
		})
	}
}

type cancelMigrationSource struct {
	legacy.PlatformMigrationSource
	cancel context.CancelFunc
}

func (s cancelMigrationSource) DeleteMigratedPlatform(ctx context.Context, id string) error {
	if err := s.PlatformMigrationSource.DeleteMigratedPlatform(ctx, id); err != nil {
		return err
	}
	s.cancel()
	return nil
}

func TestPlatformTokenMigrationCancellationAfterWriteRollsBack(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedLegacyPlatform(t, db, 0, true)
	seedLegacyPlatform(t, db, 1, false)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := cancelMigrationSource{legacyrepo.NewPlatformMigrationSource(db.Gorm), cancel}
	flow := workflow.NewPlatformTokenMigration(dal.NewTxRunner(db.Gorm), source, platformrepo.NewRepo(db.Gorm), platformTestRing(t))
	if _, err := flow.Run(ctx, true); !errors.Is(err, workflow.ErrPlatformMigration) {
		t.Fatal("cancelled transfer succeeded")
	}
	migrationCounts(t, db, 2, 0)
}

func TestPlatformTokenMigrationMaintenanceExecutableDefaultsToRollback(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedLegacyPlatform(t, db, 0, false)
	dir := t.TempDir()
	keyPath, configPath, binary := filepath.Join(dir, "key"), filepath.Join(dir, "config.yaml"), filepath.Join(dir, "migrate")
	if err := os.WriteFile(keyPath, bytes.Repeat([]byte{7}, 32), 0600); err != nil {
		t.Fatal("test Secret file failed")
	}
	config := fmt.Sprintf("database:\n  enabled: true\n  driver: postgres\n  dsn: %q\n", db.DSN)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal("test config file failed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./tools/platform-token-migrate")
	build.Dir = ".."
	if err := build.Run(); err != nil {
		t.Fatal("maintenance tool build failed")
	}
	invoke := func(extra ...string) workflow.PlatformMigrationResult {
		t.Helper()
		args := []string{"--config", configPath, "--key-file", keyPath, "--key-version", "test-cli-v1"}
		cmd := exec.CommandContext(ctx, binary, append(args, extra...)...)
		var out, diagnostics bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &diagnostics
		if err := cmd.Run(); err != nil || diagnostics.Len() != 0 {
			t.Fatal("maintenance tool execution failed")
		}
		var result workflow.PlatformMigrationResult
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal("maintenance tool returned non-count output")
		}
		return result
	}
	if r := invoke(); r.Committed || r.Verified != 1 {
		t.Fatal("default invocation was not a full dry run")
	}
	migrationCounts(t, db, 1, 0)
	if r := invoke("--apply"); !r.Committed || r.Created != 1 || r.Verified != 1 {
		t.Fatal("explicit apply did not transfer source")
	}
	migrationCounts(t, db, 0, 1)
	if r := invoke("--apply"); !r.Committed || r.Verified != 0 {
		t.Fatal("CLI rerun was not idempotent")
	}
	bad := exec.CommandContext(ctx, binary, "--config", configPath, "--key-file", keyPath, "--key-version", "v1", "--unknown=must-not-echo")
	var out, diagnostics bytes.Buffer
	bad.Stdout, bad.Stderr = &out, &diagnostics
	if err := bad.Run(); err == nil || out.Len() != 0 || diagnostics.String() != "platform token migration failed; verify maintenance prerequisites\n" {
		t.Fatal("invalid invocation did not return fixed safe error")
	}
}

//go:build e2e

package e2e

import (
	"context"
	"errors"
	"testing"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_auth/biz/dal"
	authrepo "media_agent/media_auth/biz/dal/repo/auth"
	platformrepo "media_agent/media_auth/biz/dal/repo/platform_token"
	legacyrepo "media_agent/media_auth/biz/dal/repo/service_token"
	domain "media_agent/media_auth/biz/domain/platform_token"
	authservice "media_agent/media_auth/biz/service/auth"
	"media_agent/media_auth/biz/shared/platformcipher"
	"media_agent/media_auth/biz/shared/session"
	"media_agent/media_auth/biz/workflow"
)

func TestPlatformTokenReadinessRejectsUnmigratedDirtyAndCorruptHistory(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	ring, repo := platformTestRing(t), platformrepo.NewRepo(db.Gorm)
	check := func(want bool) {
		t.Helper()
		err := workflow.CheckPlatformCustody(t.Context(), repo, ring)
		if (err == nil) != want {
			t.Fatal("unexpected cutover readiness")
		}
	}
	// The shared fixture executes SQL files directly, not golang-migrate.
	// Missing migration metadata must fail, then supply its explicit fixture.
	check(false)
	if err := db.Gorm.Exec("CREATE TABLE media_auth.schema_migrations(version bigint PRIMARY KEY, dirty boolean NOT NULL); INSERT INTO media_auth.schema_migrations VALUES (16,false)").Error; err != nil {
		t.Fatal("migration metadata fixture failed")
	}
	check(true)
	for i := 0; i < 105; i++ {
		seedLegacyPlatform(t, db, i, true)
	}
	check(false)
	flow := workflow.NewPlatformTokenMigration(dal.NewTxRunner(db.Gorm), legacyrepo.NewPlatformMigrationSource(db.Gorm), repo, ring)
	if result, err := flow.Run(t.Context(), true); err != nil || result.Verified != 105 || !result.Committed {
		t.Fatal("isolated migration failed")
	}
	check(true)
	if err := db.Gorm.Exec(`INSERT INTO media_auth.service_tokens(id,name,token_hash,created_by,is_platform) VALUES ('generic','keep','generic-hash','owner',false)`).Error; err != nil {
		t.Fatal("generic fixture failed")
	}
	check(true)
	if err := db.Gorm.Exec("UPDATE media_auth.schema_migrations SET dirty=true").Error; err != nil {
		t.Fatal("dirty fixture failed")
	}
	check(false)
	if err := db.Gorm.Exec("UPDATE media_auth.schema_migrations SET dirty=false").Error; err != nil {
		t.Fatal("dirty reset failed")
	}
	check(true)
	wrong, err := platformcipher.New("wrong-version", map[string][]byte{"wrong-version": make([]byte, 32)})
	if err != nil {
		t.Fatal("fixture key failed")
	}
	if err := workflow.CheckPlatformCustody(t.Context(), repo, wrong); err == nil {
		t.Fatal("missing historical key passed startup")
	}
	if err := db.Gorm.Exec("UPDATE media_auth.platform_tokens SET ciphertext=set_byte(ciphertext,0,get_byte(ciphertext,0) # 1) WHERE id='legacy-0104'").Error; err != nil {
		t.Fatal("corruption fixture failed")
	}
	check(false)
	migrationCounts(t, db, 0, 105)
}

type localCredentialCleanup struct{ err error }

func (c *localCredentialCleanup) RevokeUserCredentials(context.Context, string) error { return c.err }
func (c *localCredentialCleanup) DeleteTokensForUser(context.Context, string) error   { return c.err }

func TestPlatformTokenKratosBootstrapAndUserRevocationTransaction(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "admin", "admin")
	if err := db.Gorm.Exec("UPDATE media_auth.users SET email='owner@example.test' WHERE id='owner'").Error; err != nil {
		t.Fatal("identity fixture failed")
	}
	users := authrepo.NewRepo(db.Gorm)
	custody := platformCore(t, db, nil)
	signer, err := session.NewSigner(&configpb.AuthConfig{JwtSecret: "isolated-fixture-key", JwtExpiresInSeconds: 3600})
	if err != nil {
		t.Fatal("fixture signer failed")
	}
	auth := authservice.NewService(users, signer, nil)
	auth.SetPlatformTokenEnsurer(custody)
	user, err := auth.ResolveKratosIdentity(t.Context(), "fixture-identity", "owner@example.test", "Owner", false)
	if err != nil || user.ID != "owner" {
		t.Fatal("Kratos business mapping failed")
	}
	migrationCounts(t, db, 0, 1)
	platformAuditCount(t, db, 1)
	if _, err := auth.ResolveKratosIdentity(t.Context(), "fixture-identity", "owner@example.test", "Owner", false); err != nil {
		t.Fatal("repeat mapping failed")
	}
	platformAuditCount(t, db, 1)
	repo := platformrepo.NewRepo(db.Gorm)
	original, err := repo.GetCurrent(t.Context(), "owner")
	if err != nil {
		t.Fatal("bootstrap custody missing")
	}
	outbox := authrepo.NewAccessRevocationOutbox(db.Gorm)
	mcp := &localCredentialCleanup{err: errors.New("fixture local cleanup failure")}
	flow := workflow.NewUserSoftDelete(users, outbox, legacyrepo.NewRepo(db.Gorm), &localCredentialCleanup{}, mcp, dal.NewTxRunner(db.Gorm))
	flow.SetPlatformTokenRevoker(custody)
	if err := flow.UpdateStatus(t.Context(), "admin", "owner", "disabled"); err != nil {
		t.Fatal("disable failed")
	}
	if err := flow.ProcessPending(t.Context()); err != nil {
		t.Fatal("failure was not scheduled")
	}
	if current, err := repo.GetCurrent(t.Context(), "owner"); err != nil || current.ID != original.ID {
		t.Fatal("failed cleanup partially revoked custody")
	}
	platformAuditCount(t, db, 1)
	var delivered int64
	if db.Gorm.Table("media_auth.user_access_outbox").Where("status='delivered'").Count(&delivered).Error != nil || delivered != 0 {
		t.Fatal("failed event confirmed")
	}
	mcp.err = nil
	if err := db.Gorm.Exec("UPDATE media_auth.user_access_outbox SET available_at=now() WHERE status='pending'").Error; err != nil {
		t.Fatal("retry fixture failed")
	}
	if err := flow.ProcessPending(t.Context()); err != nil {
		t.Fatal("cleanup retry failed")
	}
	if _, err := repo.GetCurrent(t.Context(), "owner"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("independent custody not revoked")
	}
	platformAuditCount(t, db, 2)
	if db.Gorm.Table("media_auth.user_access_outbox").Where("status='delivered'").Count(&delivered).Error != nil || delivered != 1 {
		t.Fatal("successful event not confirmed")
	}
	if err := custody.EnsurePlatformToken(t.Context(), "owner"); err == nil {
		t.Fatal("late disabled bootstrap recreated custody")
	}
	if err := flow.ProcessPending(t.Context()); err != nil {
		t.Fatal("repeat cleanup failed")
	}
	platformAuditCount(t, db, 2)
	if err := flow.UpdateStatus(t.Context(), "admin", "owner", "active"); err != nil {
		t.Fatal("reactivation failed")
	}
	if err := custody.EnsurePlatformToken(t.Context(), "owner"); err != nil {
		t.Fatal("reactivated bootstrap failed")
	}
	if current, err := repo.GetCurrent(t.Context(), "owner"); err != nil || current.ID == original.ID {
		t.Fatal("reactivation reused revoked custody")
	}
}

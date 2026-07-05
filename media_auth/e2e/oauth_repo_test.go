//go:build e2e

// oauth_repo_test.go：oauth 仓储集成测试（testcontainers postgres）。
//
// 覆盖关键不变量：auth_code 一次性（ConsumeAuthCode 删除 + 重复消费 → not found）、
//
//	refresh_token 一次性 + 轮转、过期 code/refresh 不消费、CleanupExpired。
package e2e

import (
	"context"
	"errors"
	"testing"
	"time"

	daloauth "media_agent/media_auth/biz/dal/repo/oauth"
	domainoauth "media_agent/media_auth/biz/domain/oauth"
	"media_agent/media_auth/biz/shared/token"
)

func newOAuthRepo(t *testing.T) domainoauth.OAuthRepo {
	db := newDB(t)
	seedUser(t, db, "u1", "alice")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.oauth_clients (id, name, redirect_uris, created_by) VALUES ('c1', 'C1', ARRAY['https://app/cb'], 'u1')`).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	return daloauth.NewRepo(db.Gorm)
}

func TestOAuthRepo_ConsumeAuthCode_OneTime(t *testing.T) {
	repo := newOAuthRepo(t)
	future := time.Now().Add(10 * time.Minute)
	repo.CreateAuthCode(context.Background(), domainoauth.AuthCode{
		Code: "code-1", ClientID: "c1", UserID: "u1", RedirectURI: "https://app/cb", Scope: "profile", ExpiresAt: future,
	})
	ac, err := repo.ConsumeAuthCode(context.Background(), "code-1")
	if err != nil || ac.Code != "code-1" || ac.UserID != "u1" {
		t.Fatalf("first consume: got ac=%+v err=%v", ac, err)
	}
	_, err = repo.ConsumeAuthCode(context.Background(), "code-1")
	if !errors.Is(err, domainoauth.ErrAuthCodeNotFound) {
		t.Errorf("replay: want ErrAuthCodeNotFound, got %v", err)
	}
}

func TestOAuthRepo_ConsumeAuthCode_Expired(t *testing.T) {
	repo := newOAuthRepo(t)
	past := time.Now().Add(-time.Minute)
	repo.CreateAuthCode(context.Background(), domainoauth.AuthCode{
		Code: "code-exp", ClientID: "c1", UserID: "u1", RedirectURI: "https://app/cb", ExpiresAt: past,
	})
	_, err := repo.ConsumeAuthCode(context.Background(), "code-exp")
	if !errors.Is(err, domainoauth.ErrAuthCodeNotFound) {
		t.Errorf("expired code: want ErrAuthCodeNotFound, got %v", err)
	}
}

func TestOAuthRepo_ConsumeRefreshToken_OneTimeAndRotation(t *testing.T) {
	repo := newOAuthRepo(t)
	hash := token.HashToken("rt-1")
	future := time.Now().Add(30 * 24 * time.Hour)
	if err := repo.CreateRefreshToken(context.Background(), domainoauth.RefreshToken{
		TokenHash: hash, ClientID: "c1", UserID: "u1", ExpiresAt: future,
	}); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	rt, err := repo.ConsumeRefreshToken(context.Background(), hash)
	if err != nil || rt.UserID != "u1" {
		t.Fatalf("first consume: got rt=%+v err=%v", rt, err)
	}
	_, err = repo.ConsumeRefreshToken(context.Background(), hash)
	if !errors.Is(err, domainoauth.ErrRefreshTokenNotFound) {
		t.Errorf("replay refresh: want ErrRefreshTokenNotFound, got %v", err)
	}
	// 轮转后新 refresh 可消费。
	hash2 := token.HashToken("rt-2")
	repo.CreateRefreshToken(context.Background(), domainoauth.RefreshToken{
		TokenHash: hash2, ClientID: "c1", UserID: "u1", ExpiresAt: future,
	})
	if _, err := repo.ConsumeRefreshToken(context.Background(), hash2); err != nil {
		t.Errorf("consume rotated refresh: %v", err)
	}
}

func TestOAuthRepo_GetClient(t *testing.T) {
	repo := newOAuthRepo(t)
	c, err := repo.GetClient(context.Background(), "c1")
	if err != nil || c.ID != "c1" || len(c.RedirectURIs) != 1 {
		t.Errorf("GetClient: got c=%+v err=%v", c, err)
	}
	_, err = repo.GetClient(context.Background(), "nope")
	if !errors.Is(err, domainoauth.ErrClientNotFound) {
		t.Errorf("missing client: want ErrClientNotFound, got %v", err)
	}
}

func TestOAuthRepo_CleanupExpired(t *testing.T) {
	repo := newOAuthRepo(t)
	past := time.Now().Add(-time.Minute)
	repo.CreateAuthCode(context.Background(), domainoauth.AuthCode{
		Code: "old-code", ClientID: "c1", UserID: "u1", RedirectURI: "https://app/cb", ExpiresAt: past,
	})
	hash := token.HashToken("old-rt")
	repo.CreateRefreshToken(context.Background(), domainoauth.RefreshToken{
		TokenHash: hash, ClientID: "c1", UserID: "u1", ExpiresAt: past,
	})
	if err := repo.CleanupExpired(context.Background()); err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	_, err := repo.ConsumeAuthCode(context.Background(), "old-code")
	if !errors.Is(err, domainoauth.ErrAuthCodeNotFound) {
		t.Errorf("after cleanup: want ErrAuthCodeNotFound, got %v", err)
	}
}

// ── OAuth client 管理 ────────────────────────────────────────────────

func TestOAuthRepo_CreateListDeleteClient(t *testing.T) {
	repo := newOAuthRepo(t)
	c, err := repo.CreateClient(context.Background(), domainoauth.Client{
		ID: "cid-new", Name: "New Client", SecretHash: "hash", RedirectURIs: []string{"https://app/cb"}, CreatedBy: "u1",
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if c.ID != "cid-new" || c.Name != "New Client" {
		t.Errorf("got c=%+v", c)
	}
	// 回读（created_at 由 DB 默认）。
	got, err := repo.GetClient(context.Background(), "cid-new")
	if err != nil || got.CreatedAt.IsZero() {
		t.Errorf("GetClient after create: got=%+v err=%v (created_at should be set by DB)", got, err)
	}
	// ListClients 含新 client。
	clients, err := repo.ListClients(context.Background())
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	found := false
	for _, cl := range clients {
		if cl.ID == "cid-new" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListClients missing new client")
	}
	// DeleteClient。
	if err := repo.DeleteClient(context.Background(), "cid-new"); err != nil {
		t.Errorf("DeleteClient: %v", err)
	}
	// 再删 → ErrClientNotFound。
	if err := repo.DeleteClient(context.Background(), "cid-new"); !errors.Is(err, domainoauth.ErrClientNotFound) {
		t.Errorf("re-delete: want ErrClientNotFound, got %v", err)
	}
}

func TestOAuthRepo_DeleteClient_Cascades(t *testing.T) {
	repo := newOAuthRepo(t)
	// 建 client + 关联 auth_code + refresh_token。
	repo.CreateClient(context.Background(), domainoauth.Client{
		ID: "cid-cascade", Name: "C", RedirectURIs: []string{"https://app/cb"}, CreatedBy: "u1",
	})
	future := time.Now().Add(10 * time.Minute)
	repo.CreateAuthCode(context.Background(), domainoauth.AuthCode{
		Code: "code-x", ClientID: "cid-cascade", UserID: "u1", RedirectURI: "https://app/cb", ExpiresAt: future,
	})
	hash := token.HashToken("rt-x")
	repo.CreateRefreshToken(context.Background(), domainoauth.RefreshToken{
		TokenHash: hash, ClientID: "cid-cascade", UserID: "u1", ExpiresAt: future,
	})
	// 删 client → auth_code + refresh_token 应级联清。
	if err := repo.DeleteClient(context.Background(), "cid-cascade"); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}
	_, err := repo.ConsumeAuthCode(context.Background(), "code-x")
	if !errors.Is(err, domainoauth.ErrAuthCodeNotFound) {
		t.Errorf("cascade: auth_code should be gone, got err=%v", err)
	}
	_, err = repo.ConsumeRefreshToken(context.Background(), hash)
	if !errors.Is(err, domainoauth.ErrRefreshTokenNotFound) {
		t.Errorf("cascade: refresh_token should be gone, got err=%v", err)
	}
}

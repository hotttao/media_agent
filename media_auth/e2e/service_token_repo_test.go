//go:build e2e

// service_token_repo_test.go：service_token 仓储集成测试（testcontainers postgres）。
//
// 覆盖关键不变量：每用户至多一个 active platform token（EnsurePlatformToken 幂等）、
//
//	GetByHash（VerifyToken 用）、Revoke 仅非 platform、ListByUser。
package e2e

import (
	"context"
	"errors"
	"testing"

	dalst "media_agent/media_auth/biz/dal/repo/service_token"
	domainst "media_agent/media_auth/biz/domain/service_token"
	"media_agent/media_auth/biz/shared/token"
)

func newSTRepo(t *testing.T) domainst.TokenRepo {
	db := newDB(t)
	seedUser(t, db, "u1", "alice")
	return dalst.NewRepo(db.Gorm)
}

func TestSTRepo_CreateAndGetByHash(t *testing.T) {
	repo := newSTRepo(t)
	hash := token.HashToken("tos_xxx")
	tok, err := repo.Create(context.Background(), "my token", hash, "u1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tok.ID == "" || tok.Name != "my token" || tok.IsPlatform {
		t.Errorf("got tok=%+v", tok)
	}
	got, err := repo.GetByHash(context.Background(), hash)
	if err != nil || got.ID != tok.ID {
		t.Errorf("GetByHash: got=%+v err=%v", got, err)
	}
	_, err = repo.GetByHash(context.Background(), "bogus")
	if !errors.Is(err, domainst.ErrNotFound) {
		t.Errorf("bogus hash: want ErrNotFound, got %v", err)
	}
}

func TestSTRepo_EnsurePlatformToken_Idempotent(t *testing.T) {
	repo := newSTRepo(t)
	pt1, err := repo.EnsurePlatformToken(context.Background(), "u1")
	if err != nil {
		t.Fatalf("first EnsurePlatformToken: %v", err)
	}
	if pt1 == "" {
		t.Fatalf("platform token empty")
	}
	pt2, _ := repo.EnsurePlatformToken(context.Background(), "u1")
	if pt2 != pt1 {
		t.Errorf("EnsurePlatformToken not idempotent: pt1=%q pt2=%q", pt1, pt2)
	}
}

func TestSTRepo_GetPlatformToken(t *testing.T) {
	repo := newSTRepo(t)
	if pt, _ := repo.GetPlatformToken(context.Background(), "u1"); pt != "" {
		t.Errorf("before ensure: want empty, got %q", pt)
	}
	pt, _ := repo.EnsurePlatformToken(context.Background(), "u1")
	got, _ := repo.GetPlatformToken(context.Background(), "u1")
	if got != pt {
		t.Errorf("GetPlatformToken: want %q, got %q", pt, got)
	}
}

func TestSTRepo_Revoke_OnlyNonPlatform(t *testing.T) {
	repo := newSTRepo(t)
	hash := token.HashToken("tos_1")
	tok, _ := repo.Create(context.Background(), "t1", hash, "u1")
	if err := repo.Revoke(context.Background(), tok.ID); err != nil {
		t.Errorf("Revoke non-platform: %v", err)
	}
	// 撤销后 GetByHash → not found（revoked_at IS NULL 过滤）。
	_, err := repo.GetByHash(context.Background(), hash)
	if !errors.Is(err, domainst.ErrNotFound) {
		t.Errorf("after revoke: want ErrNotFound, got %v", err)
	}
	// 再撤销同一 → not found（已撤销）。
	if err := repo.Revoke(context.Background(), tok.ID); !errors.Is(err, domainst.ErrNotFound) {
		t.Errorf("re-revoke: want ErrNotFound, got %v", err)
	}
}

func TestSTRepo_Revoke_PlatformRejected(t *testing.T) {
	repo := newSTRepo(t)
	// ensure 一个 platform token，尝试撤销 → ErrNotFound（WHERE is_platform=false 不命中）。
	pt, _ := repo.EnsurePlatformToken(context.Background(), "u1")
	// 取其 id（通过 ListByUser）。
	toks, _ := repo.ListByUser(context.Background(), "u1")
	var platformID string
	for _, tk := range toks {
		if tk.IsPlatform {
			platformID = tk.ID
		}
	}
	if platformID == "" {
		t.Fatalf("no platform token found")
	}
	if err := repo.Revoke(context.Background(), platformID); !errors.Is(err, domainst.ErrNotFound) {
		t.Errorf("revoke platform: want ErrNotFound, got %v", err)
	}
	_ = pt
}

func TestSTRepo_ListByUser(t *testing.T) {
	repo := newSTRepo(t)
	repo.Create(context.Background(), "t1", token.HashToken("a"), "u1")
	repo.Create(context.Background(), "t2", token.HashToken("b"), "u1")
	repo.EnsurePlatformToken(context.Background(), "u1")
	toks, err := repo.ListByUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(toks) != 3 { // 2 普通 + 1 platform
		t.Errorf("want 3 tokens, got %d", len(toks))
	}
}

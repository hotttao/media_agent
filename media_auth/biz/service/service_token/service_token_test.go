// service_test.go：service_token 用例 service 单测（fake repo，不依赖 DB/网络）。
//
// 覆盖：CreateServiceToken（tos_ 前缀 + hash 落库 + 明文返回）、RevokeServiceToken（委托 repo）、
//
//	EnsurePlatformToken（实现 PlatformTokenEnsurer）、GetServiceTokenByHash、GetPlatformToken。
package servicetoken

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	domainst "media_agent/media_auth/biz/domain/service_token"
	"media_agent/media_auth/biz/shared/token"
)

// fakeRepo 实现 domainst.TokenRepo（内存版）。
type fakeRepo struct {
	mu             sync.Mutex
	tokens         map[string]domainst.Token // id → token
	byHash         map[string]domainst.Token // hash → token
	platformByUser map[string]string         // userID → plaintext
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		tokens: map[string]domainst.Token{}, byHash: map[string]domainst.Token{},
		platformByUser: map[string]string{},
	}
}

func (r *fakeRepo) Create(_ context.Context, name, tokenHash, createdBy string) (domainst.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := domainst.Token{ID: "id-" + name, Name: name, TokenHash: tokenHash, CreatedBy: createdBy, CreatedAt: time.Now()}
	r.tokens[t.ID] = t
	r.byHash[tokenHash] = t
	return t, nil
}
func (r *fakeRepo) GetByHash(_ context.Context, tokenHash string) (domainst.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byHash[tokenHash]
	if !ok {
		return domainst.Token{}, domainst.ErrNotFound
	}
	return t, nil
}
func (r *fakeRepo) ListByUser(_ context.Context, userID string) ([]domainst.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domainst.Token
	for _, t := range r.tokens {
		if t.CreatedBy == userID && !t.IsRevoked() {
			out = append(out, t)
		}
	}
	return out, nil
}
func (r *fakeRepo) Revoke(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[id]
	if !ok || t.IsPlatform || t.IsRevoked() {
		return domainst.ErrNotFound
	}
	now := time.Now()
	t.RevokedAt = &now
	r.tokens[id] = t
	return nil
}
func (r *fakeRepo) GetPlatformToken(_ context.Context, userID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.platformByUser[userID], nil
}
func (r *fakeRepo) EnsurePlatformToken(_ context.Context, userID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pt, ok := r.platformByUser[userID]; ok {
		return pt, nil
	}
	pt, _, _ := token.GenerateTokenAndHash()
	r.platformByUser[userID] = pt
	return pt, nil
}
func (r *fakeRepo) RevokeAllForUser(context.Context, string) error { return nil }

// ── 用例 ──────────────────────────────────────────────────────────────

func TestCreateServiceToken(t *testing.T) {
	svc := NewService(newFakeRepo())
	res, err := svc.CreateServiceToken(context.Background(), "u1", "my token")
	if err != nil {
		t.Fatalf("CreateServiceToken: %v", err)
	}
	if !strings.HasPrefix(res.Token, "tos_") {
		t.Errorf("token should have tos_ prefix, got %q", res.Token)
	}
	if res.Name != "my token" || res.ID == "" {
		t.Errorf("got res=%+v", res)
	}
}

func TestCreateServiceToken_EmptyName(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.CreateServiceToken(context.Background(), "u1", "")
	if err != ErrNameRequired {
		t.Errorf("want ErrNameRequired, got %v", err)
	}
}

func TestRevokeServiceToken(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	res, _ := svc.CreateServiceToken(context.Background(), "u1", "tok")

	if err := svc.RevokeServiceToken(context.Background(), "u1", res.ID); err != nil {
		t.Errorf("RevokeServiceToken: %v", err)
	}
	// 再撤销 → not found（已撤销）。
	if err := svc.RevokeServiceToken(context.Background(), "u1", res.ID); err != ErrNotFound {
		t.Errorf("re-revoke: want ErrNotFound, got %v", err)
	}
}

func TestEnsurePlatformToken_Idempotent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	// 第一次 ensure → 建。
	if err := svc.EnsurePlatformToken(context.Background(), "u1"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	first, _ := repo.GetPlatformToken(context.Background(), "u1")
	if first == "" {
		t.Fatalf("platform token not created")
	}
	// 第二次 ensure → 返回同一个（幂等）。
	if err := svc.EnsurePlatformToken(context.Background(), "u1"); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	second, _ := repo.GetPlatformToken(context.Background(), "u1")
	if second != first {
		t.Errorf("platform token should be idempotent: first=%q second=%q", first, second)
	}
}

func TestGetServiceTokenByHash(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	res, _ := svc.CreateServiceToken(context.Background(), "u1", "tok")

	tok, err := svc.GetServiceTokenByHash(context.Background(), token.HashToken(res.Token))
	if err != nil || tok.CreatedBy != "u1" {
		t.Errorf("GetServiceTokenByHash: got tok=%+v err=%v", tok, err)
	}
	// 不存在的 hash → ErrNotFound。
	_, err = svc.GetServiceTokenByHash(context.Background(), "bogus-hash")
	if err != domainst.ErrNotFound {
		t.Errorf("bogus hash: want ErrNotFound, got %v", err)
	}
}

// service_test.go：identity 用例 service 单测（fake repo，不依赖 DB/网络）。
//
// 覆盖：FindUserIDentity（命中/未命中→""）、BindIdentity（already_bound/success）、
//
//	ResolveByUserID（实现 AuthSourceResolver：有 wecom→wecom，无→""）。
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	domainidentity "media_agent/media_auth/biz/domain/identity"
)

// fakeRepo 实现 domainidentity.IdentityRepo（内存版）。
type fakeRepo struct {
	mu         sync.Mutex
	byExternal map[string]domainidentity.Identity // key: provider+":"+external_id
	byUser     map[string]map[string]domainidentity.Identity
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byExternal: map[string]domainidentity.Identity{},
		byUser:     map[string]map[string]domainidentity.Identity{},
	}
}

func key(provider, externalID string) string { return provider + ":" + externalID }

func (r *fakeRepo) ListByUser(_ context.Context, userID string) ([]domainidentity.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domainidentity.Identity, 0)
	for _, id := range r.byUser[userID] {
		out = append(out, id)
	}
	return out, nil
}
func (r *fakeRepo) FindByExternal(_ context.Context, provider domainidentity.Provider, externalID string) (domainidentity.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byExternal[key(string(provider), externalID)]
	if !ok {
		return domainidentity.Identity{}, domainidentity.ErrNotFound
	}
	return id, nil
}
func (r *fakeRepo) FindByUserProvider(_ context.Context, userID string, provider domainidentity.Provider) (domainidentity.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byUser[userID][string(provider)]
	if !ok {
		return domainidentity.Identity{}, domainidentity.ErrNotFound
	}
	return id, nil
}
func (r *fakeRepo) Create(_ context.Context, id domainidentity.Identity) (domainidentity.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byExternal[key(string(id.Provider), id.ExternalID)]; exists {
		return domainidentity.Identity{}, domainidentity.ErrAlreadyBound
	}
	r.byExternal[key(string(id.Provider), id.ExternalID)] = id
	if r.byUser[id.UserID] == nil {
		r.byUser[id.UserID] = map[string]domainidentity.Identity{}
	}
	r.byUser[id.UserID][string(id.Provider)] = id
	return id, nil
}
func (r *fakeRepo) Upsert(_ context.Context, id domainidentity.Identity) (domainidentity.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// 模拟 ON CONFLICT (user_id, provider) DO UPDATE。
	if existing, exists := r.byExternal[key(string(id.Provider), id.ExternalID)]; exists && existing.UserID != id.UserID {
		return domainidentity.Identity{}, domainidentity.ErrAlreadyBound
	}
	r.byExternal[key(string(id.Provider), id.ExternalID)] = id
	if r.byUser[id.UserID] == nil {
		r.byUser[id.UserID] = map[string]domainidentity.Identity{}
	}
	r.byUser[id.UserID][string(id.Provider)] = id
	return id, nil
}
func (r *fakeRepo) Delete(_ context.Context, userID string, provider domainidentity.Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byUser[userID][string(provider)]
	if !ok {
		return domainidentity.ErrNotFound
	}
	delete(r.byUser[userID], string(provider))
	delete(r.byExternal, key(string(provider), id.ExternalID))
	return nil
}

// ── 用例 ──────────────────────────────────────────────────────────────

func TestFindUserIDentity(t *testing.T) {
	repo := newFakeRepo()
	repo.byExternal["wecom:ext1"] = domainidentity.Identity{UserID: "u1", Provider: "wecom", ExternalID: "ext1"}
	svc := NewService(repo)

	// 命中。
	uid, err := svc.FindUserIDentity(context.Background(), "wecom", "ext1")
	if err != nil || uid != "u1" {
		t.Errorf("hit: want u1, got uid=%q err=%v", uid, err)
	}
	// 未命中 → ""（不报错）。
	uid, err = svc.FindUserIDentity(context.Background(), "wecom", "nope")
	if err != nil || uid != "" {
		t.Errorf("miss: want '', got uid=%q err=%v", uid, err)
	}
}

func TestBindIdentity_AlreadyBound(t *testing.T) {
	repo := newFakeRepo()
	repo.byExternal["wecom:ext1"] = domainidentity.Identity{UserID: "u_other", Provider: "wecom", ExternalID: "ext1"}
	svc := NewService(repo)

	err := svc.BindIdentity(context.Background(), "u1", "wecom", "ext1", "Alice", nil)
	if !errors.Is(err, ErrAlreadyBound) {
		t.Errorf("already bound: want ErrAlreadyBound, got %v", err)
	}
}

func TestBindIdentity_Success(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	meta := json.RawMessage(`{"avatar":"http://x/a.png"}`)
	if err := svc.BindIdentity(context.Background(), "u1", "wecom", "ext1", "Alice", meta); err != nil {
		t.Fatalf("BindIdentity: %v", err)
	}
	// 验证落库。
	got, err := repo.FindByExternal(context.Background(), "wecom", "ext1")
	if err != nil || got.UserID != "u1" || got.DisplayName != "Alice" {
		t.Errorf("after bind: got=%+v err=%v", got, err)
	}
}

func TestBindIdentity_SameUserUpsert(t *testing.T) {
	// 同一 user 重复 bind 同 provider → upsert（不报 already_bound）。
	repo := newFakeRepo()
	repo.byExternal["wecom:ext1"] = domainidentity.Identity{UserID: "u1", Provider: "wecom", ExternalID: "ext1", DisplayName: "Old"}
	svc := NewService(repo)
	if err := svc.BindIdentity(context.Background(), "u1", "wecom", "ext1", "New", nil); err != nil {
		t.Errorf("same user upsert: %v", err)
	}
}

func TestResolveByUserID(t *testing.T) {
	repo := newFakeRepo()
	repo.byUser["u_wecom"] = map[string]domainidentity.Identity{
		"wecom": {UserID: "u_wecom", Provider: "wecom", ExternalID: "ext"},
	}
	svc := NewService(repo)
	if got := svc.ResolveByUserID(context.Background(), "u_wecom"); got != "wecom" {
		t.Errorf("with wecom identity: want wecom, got %q", got)
	}
	if got := svc.ResolveByUserID(context.Background(), "u_ldap"); got != "" {
		t.Errorf("without identity: want '', got %q", got)
	}
}

func TestListIdentities(t *testing.T) {
	repo := newFakeRepo()
	repo.byUser["u1"] = map[string]domainidentity.Identity{
		"wecom": {UserID: "u1", Provider: "wecom", ExternalID: "ext1", DisplayName: "Alice", CreatedAt: time.Now()},
	}
	svc := NewService(repo)
	ids, err := svc.ListIdentities(context.Background(), "u1")
	if err != nil || len(ids) != 1 || ids[0].Provider != "wecom" {
		t.Errorf("ListIdentities: got ids=%+v err=%v", ids, err)
	}
}

func TestDeleteIdentity(t *testing.T) {
	repo := newFakeRepo()
	repo.byUser["u1"] = map[string]domainidentity.Identity{
		"wecom": {UserID: "u1", Provider: "wecom", ExternalID: "ext1"},
	}
	repo.byExternal["wecom:ext1"] = domainidentity.Identity{UserID: "u1", Provider: "wecom", ExternalID: "ext1"}
	svc := NewService(repo)
	if err := svc.DeleteIdentity(context.Background(), "u1", "wecom"); err != nil {
		t.Errorf("DeleteIdentity: %v", err)
	}
	// 再删 → ErrNotFound。
	if err := svc.DeleteIdentity(context.Background(), "u1", "wecom"); !errors.Is(err, ErrNotFound) {
		t.Errorf("re-delete: want ErrNotFound, got %v", err)
	}
}

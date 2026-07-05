//go:build e2e

// identity_repo_test.go：identity 仓储集成测试（testcontainers postgres）。
//
// 覆盖双唯一不变量：(provider, external_id) PK 全局唯一；(user_id, provider) 每用户每 provider 至多一条。
//
//	Create 重复 PK → ErrAlreadyBound；Upsert 跨 user 占用 PK → ErrAlreadyBound；
//	Upsert 同 user 同 provider → 更新 external_id；Delete 0 行 → ErrNotFound。
package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	dalidentity "media_agent/media_auth/biz/dal/repo/identity"
	domainidentity "media_agent/media_auth/biz/domain/identity"
)

func newIdentityRepo(t *testing.T) domainidentity.IdentityRepo {
	db := newDB(t)
	seedUser(t, db, "u1", "alice")
	seedUser(t, db, "u2", "bob")
	return dalidentity.NewRepo(db.Gorm)
}

func TestIdentityRepo_Create_AndFindByExternal(t *testing.T) {
	repo := newIdentityRepo(t)
	_, err := repo.Create(context.Background(), domainidentity.Identity{
		UserID: "u1", Provider: "wecom", ExternalID: "ext1", DisplayName: "Alice",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// FindByExternal 命中。
	got, err := repo.FindByExternal(context.Background(), "wecom", "ext1")
	if err != nil || got.UserID != "u1" || got.DisplayName != "Alice" {
		t.Errorf("FindByExternal: got=%+v err=%v", got, err)
	}
	// FindByExternal 未命中 → ErrNotFound。
	_, err = repo.FindByExternal(context.Background(), "wecom", "nope")
	if !errors.Is(err, domainidentity.ErrNotFound) {
		t.Errorf("miss: want ErrNotFound, got %v", err)
	}
}

func TestIdentityRepo_Create_DuplicateExternal(t *testing.T) {
	repo := newIdentityRepo(t)
	repo.Create(context.Background(), domainidentity.Identity{UserID: "u1", Provider: "wecom", ExternalID: "ext1"})
	// 同 (provider, external_id) 不同 user → ErrAlreadyBound（PK 冲突）。
	_, err := repo.Create(context.Background(), domainidentity.Identity{UserID: "u2", Provider: "wecom", ExternalID: "ext1"})
	if !errors.Is(err, domainidentity.ErrAlreadyBound) {
		t.Errorf("duplicate external: want ErrAlreadyBound, got %v", err)
	}
}

func TestIdentityRepo_Upsert_SameUserSameProvider(t *testing.T) {
	repo := newIdentityRepo(t)
	// u1 先绑 wecom:ext1。
	repo.Create(context.Background(), domainidentity.Identity{UserID: "u1", Provider: "wecom", ExternalID: "ext1", DisplayName: "Old"})
	// u1 再 Upsert wecom:ext2（同 user 同 provider）→ 更新 external_id。
	_, err := repo.Upsert(context.Background(), domainidentity.Identity{
		UserID: "u1", Provider: "wecom", ExternalID: "ext2", DisplayName: "New",
	})
	if err != nil {
		t.Fatalf("Upsert same user: %v", err)
	}
	// FindByUserProvider 命中 ext2。
	got, err := repo.FindByUserProvider(context.Background(), "u1", "wecom")
	if err != nil || got.ExternalID != "ext2" || got.DisplayName != "New" {
		t.Errorf("after upsert: got=%+v err=%v", got, err)
	}
	// 旧 ext1 应被覆盖（FindByExternal 不再命中 ext1 for u1；但 PK 是 (provider,external_id)，
	// ext1 行可能仍在——取决于 Upsert 是否删旧行。当前 Upsert 是 ON CONFLICT (user_id,provider) DO UPDATE，
	// 不删旧 ext1 行。验证 ext1 仍存在但属 u1（external_id 改了，ext1 行的 external_id 还是 ext1）。
	// 实际 Upsert 把 (u1, wecom) 行的 external_id 从 ext1 改成 ext2 → ext1 行不再存在。
	// 重新查 ext1 → not found。
	_, err = repo.FindByExternal(context.Background(), "wecom", "ext1")
	if !errors.Is(err, domainidentity.ErrNotFound) {
		t.Errorf("ext1 should be gone after upsert to ext2: got err=%v", err)
	}
}

func TestIdentityRepo_Upsert_CrossUserAlreadyBound(t *testing.T) {
	repo := newIdentityRepo(t)
	// u1 绑 wecom:ext1。
	repo.Create(context.Background(), domainidentity.Identity{UserID: "u1", Provider: "wecom", ExternalID: "ext1"})
	// u2 Upsert wecom:ext1 → (provider, external_id) PK 冲突 → ErrAlreadyBound。
	_, err := repo.Upsert(context.Background(), domainidentity.Identity{
		UserID: "u2", Provider: "wecom", ExternalID: "ext1",
	})
	if !errors.Is(err, domainidentity.ErrAlreadyBound) {
		t.Errorf("cross-user upsert: want ErrAlreadyBound, got %v", err)
	}
}

func TestIdentityRepo_ListByUser(t *testing.T) {
	repo := newIdentityRepo(t)
	meta := json.RawMessage(`{"avatar":"http://x/a.png"}`)
	repo.Create(context.Background(), domainidentity.Identity{UserID: "u1", Provider: "wecom", ExternalID: "ext1", Metadata: meta})
	repo.Create(context.Background(), domainidentity.Identity{UserID: "u1", Provider: "github", ExternalID: "gh1"})

	ids, err := repo.ListByUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("want 2 identities, got %d", len(ids))
	}
}

func TestIdentityRepo_Delete(t *testing.T) {
	repo := newIdentityRepo(t)
	repo.Create(context.Background(), domainidentity.Identity{UserID: "u1", Provider: "wecom", ExternalID: "ext1"})
	// 删命中。
	if err := repo.Delete(context.Background(), "u1", "wecom"); err != nil {
		t.Errorf("Delete: %v", err)
	}
	// 再删 → ErrNotFound。
	if err := repo.Delete(context.Background(), "u1", "wecom"); !errors.Is(err, domainidentity.ErrNotFound) {
		t.Errorf("re-delete: want ErrNotFound, got %v", err)
	}
}

package material

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	domain "media_agent/media_materials/biz/domain/material"
)

// —— mock repos ——

type mockRepo struct {
	items map[string]*domain.Material
}

func (m *mockRepo) Save(_ context.Context, mat *domain.Material) error {
	mat.CreatedAt = time.Now()
	mat.UpdatedAt = time.Now()
	m.items[mat.ID] = mat
	return nil
}
func (m *mockRepo) Get(_ context.Context, id string) (*domain.Material, error) {
	mat, ok := m.items[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return mat, nil
}
func (m *mockRepo) List(_ context.Context, q domain.ListQuery) ([]*domain.Material, int, error) {
	var out []*domain.Material
	for _, mat := range m.items {
		out = append(out, mat)
	}
	return out, len(out), nil
}
func (m *mockRepo) Update(_ context.Context, mat *domain.Material) error {
	m.items[mat.ID] = mat
	return nil
}
func (m *mockRepo) Delete(_ context.Context, id string) error {
	delete(m.items, id)
	return nil
}

type mockBindingRepo struct {
	bindings map[string]*domain.Binding // key = materialID+ownerKind+ownerID
}

func (m *mockBindingRepo) AddBinding(_ context.Context, b *domain.Binding) error {
	key := b.MaterialID + string(b.OwnerKind) + b.OwnerID
	m.bindings[key] = b
	return nil
}
func (m *mockBindingRepo) RemoveBinding(_ context.Context, materialID string, ownerKind domain.BindingOwnerKind, ownerID string) error {
	key := materialID + string(ownerKind) + ownerID
	if _, ok := m.bindings[key]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.bindings, key)
	return nil
}
func (m *mockBindingRepo) ListByOwner(_ context.Context, ownerKind domain.BindingOwnerKind, ownerID string) ([]*domain.Material, error) {
	return nil, nil
}

// mockAssetKindLookup 返回固定 asset_kind 信息。
type mockAssetKindLookup struct{}

func (m *mockAssetKindLookup) Lookup(_ context.Context, name string) (string, string, string, error) {
	kinds := map[string][3]string{
		"scene":         {"MATERIAL", "IMAGE", "SYSTEM"},
		"pose":          {"MATERIAL", "IMAGE", "SYSTEM"},
		"model_image":   {"ASSET", "IMAGE", "SYSTEM"},
		"product_image": {"MATERIAL", "IMAGE", "SYSTEM"},
	}
	info, ok := kinds[name]
	if !ok {
		return "", "", "", errors.New("not found")
	}
	return info[0], info[1], info[2], nil
}

func newTestService() *Service {
	return NewService(&mockRepo{items: map[string]*domain.Material{}}, &mockBindingRepo{bindings: map[string]*domain.Binding{}}, &mockAssetKindLookup{}, nil)
}

// —— tests ——

func TestCreate_InvalidAssetKind(t *testing.T) {
	s := newTestService()
	_, err := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "nonexistent",
		URL: "https://x.png", OwnerUserID: "u1",
	})
	if err == nil {
		t.Fatal("expected error for invalid asset_kind")
	}
}

func TestCreate_WrongResourceType(t *testing.T) {
	s := newTestService()
	_, err := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "model_image", // ASSET, not MATERIAL
		URL: "https://x.png", OwnerUserID: "u1",
	})
	if err == nil {
		t.Fatal("expected error for ASSET kind on material")
	}
}

func TestCreate_DisabledAssetKind(t *testing.T) {
	s := newTestService()
	// 模拟 disabled kind：override lookup
	s.kindLookup = &disabledLookup{}
	_, err := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "scene",
		URL: "https://x.png", OwnerUserID: "u1",
	})
	if err == nil {
		t.Fatal("expected error for disabled asset_kind")
	}
}

type disabledLookup struct{}

func (d *disabledLookup) Lookup(_ context.Context, name string) (string, string, string, error) {
	return "MATERIAL", "IMAGE", "DISABLED", nil
}

func TestCreate_MissingUrlAndContent(t *testing.T) {
	s := newTestService()
	_, err := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "scene",
		OwnerUserID: "u1",
	})
	if err == nil {
		t.Fatal("expected error for missing url and content")
	}
}

func TestCreate_Success(t *testing.T) {
	s := newTestService()
	mat, err := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test_scene", AssetKind: "scene",
		URL: "https://x.png", OwnerUserID: "u1", Visibility: "private",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mat.ID == "" {
		t.Error("expected non-empty ID")
	}
	if mat.Visibility != domain.VisibilityPrivate {
		t.Errorf("visibility = %s, want private", mat.Visibility)
	}
}

func TestGet_OwnerCanRead(t *testing.T) {
	s := newTestService()
	mat, _ := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "scene",
		URL: "https://x.png", OwnerUserID: "u1",
	})
	got, err := s.Get(context.Background(), mat.ID, "u1", nil)
	if err != nil {
		t.Fatalf("owner should read: %v", err)
	}
	if got.ID != mat.ID {
		t.Error("wrong material returned")
	}
}

func TestGet_NonOwnerPrivateDenied(t *testing.T) {
	s := newTestService()
	mat, _ := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "scene",
		URL: "https://x.png", OwnerUserID: "u1", Visibility: "private",
	})
	_, err := s.Get(context.Background(), mat.ID, "u2", nil)
	if err == nil {
		t.Fatal("non-owner should not read private material")
	}
}

func TestGet_PublicReadableByAnyone(t *testing.T) {
	s := newTestService()
	mat, _ := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "scene",
		URL: "https://x.png", OwnerUserID: "u1", Visibility: "public",
	})
	_, err := s.Get(context.Background(), mat.ID, "u2", nil)
	if err != nil {
		t.Fatalf("public material should be readable by anyone: %v", err)
	}
}

func TestUpdate_OnlyOwner(t *testing.T) {
	s := newTestService()
	mat, _ := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "scene",
		URL: "https://x.png", OwnerUserID: "u1",
	})
	_, err := s.Update(context.Background(), UpdateCommand{
		ID: mat.ID, Name: "updated", CallerID: "u2",
	})
	if err == nil {
		t.Fatal("non-owner should not update")
	}
}

func TestDelete_OnlyOwner(t *testing.T) {
	s := newTestService()
	mat, _ := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "scene",
		URL: "https://x.png", OwnerUserID: "u1",
	})
	_, err := s.Delete(context.Background(), mat.ID, "u2")
	if err == nil {
		t.Fatal("non-owner should not delete")
	}
}

func TestAddBinding_InvalidOwnerKind(t *testing.T) {
	s := newTestService()
	mat, _ := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "scene",
		URL: "https://x.png", OwnerUserID: "u1",
	})
	_, err := s.AddBinding(context.Background(), AddBindingCommand{
		MaterialID: mat.ID, OwnerKind: "invalid", OwnerID: "vip1",
	})
	if err == nil {
		t.Fatal("expected error for invalid owner_kind")
	}
}

func TestAddBinding_Success(t *testing.T) {
	s := newTestService()
	mat, _ := s.Create(context.Background(), CreateCommand{
		Scope: "ai_clothing", Name: "test", AssetKind: "scene",
		URL: "https://x.png", OwnerUserID: "u1",
	})
	_, err := s.AddBinding(context.Background(), AddBindingCommand{
		MaterialID: mat.ID, OwnerKind: "virtual_ip", OwnerID: "vip1", BoundBy: "u1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

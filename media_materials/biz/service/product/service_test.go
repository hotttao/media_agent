package product

import (
	"context"
	"testing"

	domain "media_agent/media_materials/biz/domain/product"
)

type fakeRepo struct{ item *domain.Product }

func (f *fakeRepo) Save(_ context.Context, p *domain.Product) error           { f.item = p; return nil }
func (f *fakeRepo) Get(_ context.Context, id string) (*domain.Product, error) { return f.item, nil }
func (f *fakeRepo) List(_ context.Context, q domain.ListQuery) ([]*domain.Product, int, error) {
	if f.item == nil {
		return nil, 0, nil
	}
	return []*domain.Product{f.item}, 1, nil
}
func (f *fakeRepo) Update(_ context.Context, p *domain.Product) error { f.item = p; return nil }
func (f *fakeRepo) Delete(context.Context, string) error              { return nil }

func TestCreateProductRejectsMultipleMainImages(t *testing.T) {
	s := NewService(&fakeRepo{})
	_, err := s.Create(context.Background(), CreateCommand{OrganizationID: "org1", Scope: "ai_clothing", Name: "shirt", CreatedBy: "u1",
		Images: []ImageCommand{{ObjectPath: "products/a.png", IsMain: true}, {ObjectPath: "products/b.png", IsMain: true, SortOrder: 1}}})
	if err == nil {
		t.Fatal("expected multiple main images to fail")
	}
}

func TestCreateProductBuildsOrganizationOwnedImages(t *testing.T) {
	s := NewService(&fakeRepo{})
	p, err := s.Create(context.Background(), CreateCommand{OrganizationID: "org1", Scope: "ai_clothing", Name: "shirt", CreatedBy: "u1",
		Tags: `["summer"]`, Images: []ImageCommand{{ObjectPath: "products/a.png", IsMain: true}}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.OrganizationID != "org1" || len(p.Images) != 1 || p.Images[0].OrganizationID != "org1" || p.Images[0].ProductID != p.ID {
		t.Fatalf("unexpected product: %+v", p)
	}
}

func TestUpdateProductRejectsOrganizationMismatch(t *testing.T) {
	r := &fakeRepo{item: &domain.Product{ID: "p1", OrganizationID: "org1"}}
	_, err := NewService(r).Update(context.Background(), UpdateCommand{ID: "p1", OrganizationID: "org2", UpdatedBy: "u1"})
	if err == nil {
		t.Fatal("expected organization mismatch")
	}
}

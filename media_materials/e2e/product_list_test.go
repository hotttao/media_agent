//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	repo "media_agent/media_materials/biz/dal/repo/product"
	domain "media_agent/media_materials/biz/domain/product"
)

func TestProductPageImageBatchQueries(t *testing.T) {
	db := resultDatabase(t)
	tx := db.Begin()
	defer tx.Rollback()
	if tx.Error != nil {
		t.Fatal("transaction unavailable")
	}
	store := repo.NewRepo(tx)
	ctx := context.Background()
	for i := range 25 {
		org := "org"
		if i == 24 {
			org = "foreign"
		}
		id := fmt.Sprintf("product-%02d", i)
		p := &domain.Product{ID: id, OrganizationID: org, Scope: "ai_clothing", Name: id, DisplayActions: "{}", Tags: "[]", ExternalCommerce: "{}", Metadata: "{}", Status: domain.StatusActive, CreatedBy: "owner"}
		for _, order := range []int{2, 0, 1} {
			p.Images = append(p.Images, domain.Image{ID: fmt.Sprintf("image-%02d-%d", i, order), ObjectPath: fmt.Sprintf("materials/%s/%s/%d.png", org, id, order), SortOrder: order, IsMain: order == 0})
		}
		if store.Save(ctx, p) != nil {
			t.Fatal("fixture save failed")
		}
		if tx.Model(&struct{}{}).Table("media_materials.products").Where("id = ?", id).Update("updated_at", time.Unix(100+int64(i), 0)).Error != nil {
			t.Fatal("fixture order failed")
		}
	}
	var reads atomic.Int32
	if db.Callback().Query().After("gorm:query").Register("test:product-query-budget", func(*gorm.DB) { reads.Add(1) }) != nil {
		t.Fatal("query counter unavailable")
	}
	defer db.Callback().Query().Remove("test:product-query-budget")
	for _, size := range []int{1, 20, 200} {
		reads.Store(0)
		items, total, err := store.List(ctx, domain.ListQuery{OrganizationID: "org", Scope: "ai_clothing", Limit: size})
		if err != nil || total != 24 || len(items) != min(size, 24) {
			t.Fatal("page contract changed")
		}
		if reads.Load() != 3 {
			t.Fatalf("page=%d queries=%d, want constant 3", size, reads.Load())
		}
		for i, p := range items {
			if p.ID != fmt.Sprintf("product-%02d", 23-i) || p.OrganizationID != "org" || len(p.Images) != 3 {
				t.Fatal("page order or tenant isolation changed")
			}
			for n, image := range p.Images {
				if image.ProductID != p.ID || image.OrganizationID != p.OrganizationID || image.SortOrder != n || image.IsMain != (n == 0) {
					t.Fatal("image attachment or sort changed")
				}
			}
		}
	}
	reads.Store(0)
	items, total, err := store.List(ctx, domain.ListQuery{OrganizationID: "absent", Limit: 20})
	if err != nil || total != 0 || len(items) != 0 || reads.Load() != 2 {
		t.Fatal("empty page queried images or changed contract")
	}
}

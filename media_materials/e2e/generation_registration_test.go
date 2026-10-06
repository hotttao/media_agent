//go:build e2e

package e2e

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"media_agent/media_materials/biz/dal"
	repo "media_agent/media_materials/biz/dal/repo/generated_asset"
	domain "media_agent/media_materials/biz/domain/generated_asset"
	product "media_agent/media_materials/biz/domain/product"
	service "media_agent/media_materials/biz/service/generated_asset"
	"media_agent/media_materials/biz/shared/objectstorage"
)

type registrationProduct struct{}

func (registrationProduct) Get(context.Context, string) (*product.Product, error) {
	return &product.Product{ID: "product", OrganizationID: "org", Scope: "ai_clothing"}, nil
}

type registrationKinds struct{}

func (registrationKinds) Lookup(context.Context, string) (string, string, string, error) {
	return "ASSET", "IMAGE", "SYSTEM", nil
}

type registrationStorage struct {
	calls       atomic.Int64
	unavailable atomic.Bool
}

func (s *registrationStorage) Stat(context.Context, string) (objectstorage.ObjectInfo, error) {
	s.calls.Add(1)
	if s.unavailable.Load() {
		return objectstorage.ObjectInfo{}, errors.New("unavailable")
	}
	return objectstorage.ObjectInfo{SizeBytes: 42, Checksum: strings.Repeat("c", 64), ContentType: "image/png"}, nil
}
func (*registrationStorage) PresignGet(context.Context, string) (objectstorage.ResolvedObject, error) {
	return objectstorage.ResolvedObject{}, nil
}

// Actual service + repositories + PostgreSQL. Sources and Storage are controlled
// ports; this is not a running HTTP/Workflow/Conductor acceptance test.
func TestGenerationRegistrationServicePersistence(t *testing.T) {
	db := resultDatabase(t)
	ctx := context.Background()
	storage := &registrationStorage{}
	newService := func() *service.Service {
		r := repo.NewRepo(db)
		s := service.NewService(r, registrationProduct{}, nil)
		s.ConfigureResultReceipts(r, dal.NewTxRunner(db))
		s.ConfigureHistoryImport(storage, registrationKinds{})
		s.ConfigureNativeOutputStorage("https://storage.example", "media")
		return s
	}
	s := newService()
	cmd := service.UpsertCommand{Scope: "ai_clothing", AssetKind: "first_frame", Name: "frame", OwnerUserID: "user",
		URL:   "https://storage.example/media/materials/org/ai_clothing/material/frame.png?signature=temporary",
		JobID: "job", StepRunID: "step", ResultKey: strings.Repeat("a", 64), TemplateID: "tpl", TemplateVersion: 1, SourceProductID: "product"}
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := s.Upsert(ctx, cmd)
			if err != nil {
				errs <- err
				return
			}
			ids <- a.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for range errs {
		t.Fatal("concurrent service registration failed")
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if first != id {
			t.Fatal("same generation created multiple assets")
		}
	}
	var total int64
	if db.Table("media_materials.generated_assets").Count(&total).Error != nil || total != 1 {
		t.Fatal("concurrent asset count incorrect")
	}
	if db.Table("media_materials.generated_asset_results").Count(&total).Error != nil || total != 1 {
		t.Fatal("concurrent receipt count incorrect")
	}
	storage.unavailable.Store(true)
	calls := storage.calls.Load()
	cmd.URL = strings.Replace(cmd.URL, "signature=temporary", "signature=expired", 1)
	replay, err := newService().Upsert(ctx, cmd)
	if err != nil || replay.ID != first || storage.calls.Load() != calls {
		t.Fatal("new service did not reuse persisted receipt without storage access")
	}
	changed := cmd
	changed.Content = "changed"
	if _, err := s.Upsert(ctx, changed); !errors.Is(err, domain.ErrResultConflict) {
		t.Fatal("changed digest accepted")
	}
	storage.unavailable.Store(false)
	cmd.ResultKey = strings.Repeat("b", 64)
	second, err := s.Upsert(ctx, cmd)
	if err != nil || second.ID == first {
		t.Fatal("new generation did not create independent asset")
	}
	if _, err := repo.NewRepo(db).Get(ctx, first); err != nil {
		t.Fatal("old result lost")
	}
	if err := repo.NewRepo(db).Delete(ctx, second.ID); err != nil {
		t.Fatal("soft deletion failed")
	}
	if _, err := newService().Upsert(ctx, cmd); !errors.Is(err, domain.ErrResultConflict) {
		t.Fatal("deleted generation was recreated")
	}
	if db.Table("media_materials.generated_asset_results").Count(&total).Error != nil || total != 2 {
		t.Fatal("receipt lost after soft deletion")
	}
}

//go:build e2e

package e2e

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_materials/biz/dal"
	repo "media_agent/media_materials/biz/dal/repo/generated_asset"
	domain "media_agent/media_materials/biz/domain/generated_asset"
)

func resultDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16-alpine", postgres.WithDatabase("materials_e2e"), postgres.WithUsername("test"), postgres.WithPassword("test"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	if err != nil {
		t.Fatal("isolated database start failed")
	}
	t.Cleanup(func() {
		if container.Terminate(ctx) != nil {
			t.Error("isolated database cleanup failed")
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal("isolated connection unavailable")
	}
	if dal.Migrate("../migrations", &configpb.DatabaseConfig{Dsn: dsn, MigrateOnStart: true}) != nil {
		t.Fatal("isolated migration failed")
	}
	db, err := gorm.Open(pgdriver.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("isolated database unavailable")
	}
	t.Cleanup(func() { connection, _ := db.DB(); _ = connection.Close() })
	return db
}

// Isolated storage protocol; no real Materials API, Storage or Workflow calls.
func TestGenerationResultReceiptPersistence(t *testing.T) {
	db := resultDatabase(t)
	ctx := context.Background()
	store := repo.NewRepo(db)
	tx := dal.NewTxRunner(db)
	asset := func(name, step string) *domain.GeneratedAsset {
		return &domain.GeneratedAsset{ID: "gen_" + uuid.NewString(), OrganizationID: "org", Scope: "scope", Name: name, AssetKind: "image", Origin: "native", Content: "same result", OwnerUserID: "owner", Visibility: domain.VisibilityPrivate, JobID: "job", StepRunID: step, TemplateID: "template", TemplateVersion: 1}
	}
	key := strings.Repeat("a", 64)
	accept := func(resultKey, digest string, rollback bool) (string, error) {
		var accepted string
		err := tx.RunInTx(ctx, func(txCtx context.Context) error {
			prior, err := store.GetResult(txCtx, "step", resultKey, true)
			if err != nil {
				return err
			}
			if prior != nil {
				if prior.RequestDigest != digest {
					return domain.ErrResultConflict
				}
				accepted = prior.AssetID
				return nil
			}
			a := asset("result_"+resultKey, "step")
			if err := store.Save(txCtx, a); err != nil {
				return err
			}
			if err := store.SaveResult(txCtx, domain.ResultReceipt{StepRunID: "step", ResultKey: resultKey, RequestDigest: digest, AssetID: a.ID}); err != nil {
				return err
			}
			accepted = a.ID
			if rollback {
				return errors.New("injected rollback")
			}
			return nil
		})
		return accepted, err
	}
	count := func(table string) int64 {
		t.Helper()
		var n int64
		if db.Table("media_materials."+table).Count(&n).Error != nil {
			t.Fatal("count failed")
		}
		return n
	}
	if _, err := accept(key, strings.Repeat("b", 64), true); err == nil || count("generated_assets") != 0 || count("generated_asset_results") != 0 {
		t.Fatal("partial result persisted on rollback")
	}
	if _, err := store.GetResult(ctx, "step", key, true); err == nil {
		t.Fatal("key locked outside caller transaction")
	}
	if err := store.SaveResult(ctx, domain.ResultReceipt{StepRunID: "step", ResultKey: key, RequestDigest: strings.Repeat("b", 64), AssetID: "asset"}); err == nil {
		t.Fatal("receipt saved without caller transaction")
	}
	old := asset("preserved_old_asset", "step")
	if store.Save(ctx, old) != nil {
		t.Fatal("old asset setup failed")
	}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := accept(key, strings.Repeat("b", 64), false)
			ids <- id
			failures <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("concurrent acceptance failed")
		}
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id == "" || id != first {
			t.Fatal("concurrent result produced different assets")
		}
	}
	if count("generated_assets") != 2 || count("generated_asset_results") != 1 {
		t.Fatal("concurrent first registration created extra aggregate")
	}
	if _, err := accept(key, strings.Repeat("c", 64), false); !errors.Is(err, domain.ErrResultConflict) || count("generated_assets") != 2 {
		t.Fatal("same key changed result")
	}
	second, err := accept(strings.Repeat("d", 64), strings.Repeat("b", 64), false)
	if err != nil || second == first || count("generated_assets") != 3 || count("generated_asset_results") != 2 {
		t.Fatal("new generation reused old resource identity")
	}
	preserved, err := store.Get(ctx, old.ID)
	if err != nil || preserved.Content != old.Content {
		t.Fatal("existing asset was changed")
	}
	// A new repository object replays only the stored receipt.
	rebuilt := repo.NewRepo(db)
	prior, err := rebuilt.GetResult(ctx, "step", key, false)
	if err != nil || prior == nil || prior.AssetID != first {
		t.Fatal("receipt did not survive repository reconstruction")
	}
	wrongErr := tx.RunInTx(ctx, func(txCtx context.Context) error {
		return store.SaveResult(txCtx, domain.ResultReceipt{StepRunID: "wrong-step", ResultKey: key, RequestDigest: strings.Repeat("b", 64), AssetID: first})
	})
	if !errors.Is(wrongErr, domain.ErrResultConflict) {
		t.Fatal("receipt accepted wrong asset provenance")
	}
	for _, statement := range []string{
		"UPDATE media_materials.generated_asset_results SET request_digest='" + strings.Repeat("e", 64) + "' WHERE asset_id=?",
		"DELETE FROM media_materials.generated_asset_results WHERE asset_id=?",
		"UPDATE media_materials.generated_assets SET content='overwritten' WHERE id=?",
		"DELETE FROM media_materials.generated_assets WHERE id=?",
	} {
		if db.Exec(statement, first).Error == nil {
			t.Fatal("database allowed rewriting registered result")
		}
	}
	if store.Delete(ctx, first) != nil {
		t.Fatal("soft deletion rejected")
	}
	if _, err := store.Get(ctx, first); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("deleted asset remained visible")
	}
	prior, err = store.GetResult(ctx, "step", key, false)
	if err != nil || prior == nil || prior.AssetID != first {
		t.Fatal("soft deletion removed idempotency tombstone")
	}
	down, err := os.ReadFile("../migrations/000006_generation_results.down.sql")
	if err != nil {
		t.Fatal("down fixture missing")
	}
	if db.Exec(string(down)).Error == nil || count("generated_asset_results") != 2 {
		t.Fatal("downgrade discarded immutable result history")
	}
}

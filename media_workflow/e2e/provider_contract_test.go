//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	pgmigration "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	repo "media_agent/media_workflow/biz/dal/repo/capability"
)

func TestProviderCapabilityMigrationAndRegistry(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16-alpine", postgres.WithDatabase("provider_contracts"), postgres.WithUsername("test"), postgres.WithPassword("test"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	if err != nil {
		t.Fatal("isolated provider database unavailable")
	}
	t.Cleanup(func() {
		if container.Terminate(ctx) != nil {
			t.Error("isolated database cleanup failed")
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal("isolated database address unavailable")
	}
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("isolated database connection failed")
	}
	src, err := iofs.New(os.DirFS("../migrations"), ".")
	if err != nil {
		t.Fatal("migration source unavailable")
	}
	driver, err := pgmigration.WithInstance(sqlDB, &pgmigration.Config{})
	if err != nil {
		t.Fatal("migration driver unavailable")
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		t.Fatal("migration initialization failed")
	}
	t.Cleanup(func() { _, _ = m.Close() })
	if m.Migrate(22) != nil {
		t.Fatal("legacy schema initialization failed")
	}
	db, err := gorm.Open(pgdriver.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("registry database connection failed")
	}
	t.Cleanup(func() { conn, _ := db.DB(); _ = conn.Close() })
	var before string
	if db.Raw("SELECT md5(jsonb_agg(to_jsonb(c) ORDER BY capability_key)::text) FROM media_workflow.execution_capabilities c").Scan(&before).Error != nil {
		t.Fatal("legacy contract snapshot unavailable")
	}
	if m.Migrate(23) != nil {
		t.Fatal("provider migration failed")
	}
	var after string
	if db.Raw("SELECT md5(jsonb_agg(to_jsonb(c)-ARRAY['output_media_type','generation_method','generation_type'] ORDER BY capability_key)::text) FROM media_workflow.execution_capabilities c WHERE output_media_type IS NULL").Scan(&after).Error != nil || before == "" || before != after {
		t.Fatal("legacy contract changed during migration")
	}
	registry := repo.NewRepo(db)
	for _, key := range []string{"jimeng.i2i.first_frame_prompt", "jimeng.i2i.multi_image_prompt", "doubao.i2v.first_frame_prompt", "doubao.i2v.multi_image_prompt"} {
		c, err := registry.Get(ctx, key, 1)
		if err != nil || c.ValidateContract() != nil || c.Ref() != key+"@1" || c.OutputAssetKind != "" || len(c.Inputs) != 2 {
			t.Fatal("original provider mode not readable as valid contract")
		}
	}
	legacy, err := registry.Get(ctx, "media.image.model", 1)
	if err != nil || legacy.GenerationMethod != "" || legacy.GenerationType != "" || legacy.OutputMediaType != "" || legacy.OutputAssetKind != "model_image" {
		t.Fatal("legacy mode guessed or made unreadable")
	}
	for _, assignment := range []string{"output_media_type='VIDEO'", "generation_method='i2v'", "generation_type='multi_image_prompt'", "input_schema='[]'::jsonb", "provider='doubao'", "status='draft'"} {
		if db.Exec("UPDATE media_workflow.execution_capabilities SET "+assignment+" WHERE capability_key='jimeng.i2i.first_frame_prompt' AND version=1").Error == nil {
			t.Fatal("published provider contract mutation accepted")
		}
	}
	if db.Exec("DELETE FROM media_workflow.execution_capabilities WHERE capability_key='jimeng.i2i.first_frame_prompt'").Error == nil {
		t.Fatal("provider audit deleted")
	}
	if db.Exec("UPDATE media_workflow.execution_capabilities SET output_media_type='IMAGE',generation_method='i2i',generation_type='first_frame_prompt',output_asset_kind='',provider='jimeng',capability_key='jimeng.i2i.first_frame_prompt_legacy' WHERE capability_key='media.image.model'").Error == nil {
		t.Fatal("legacy contract converted in place")
	}
	insert := `INSERT INTO media_workflow.execution_capabilities(capability_key,version,provider,mode,generation_method,generation_type,input_schema,output_asset_kind,output_media_type,conductor_task_def,execution_target,retry_policy,timeout_seconds,status,published_at)
SELECT ?,2,provider,mode,generation_method,generation_type,input_schema,'',output_media_type,'jimeng_i2i_first_frame_v2',execution_target,retry_policy,timeout_seconds,'published',now() FROM media_workflow.execution_capabilities WHERE capability_key='jimeng.i2i.first_frame_prompt' AND version=1`
	if db.Exec(insert, "alias").Error == nil {
		t.Fatal("key diverged from structured identity")
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- db.Exec(insert, "jimeng.i2i.first_frame_prompt").Error }()
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatal("canonical version uniqueness failed under concurrency")
	}
	if db.Exec("UPDATE media_workflow.execution_capabilities SET status='archived' WHERE capability_key='jimeng.i2i.first_frame_prompt' AND version=2").Error != nil {
		t.Fatal("archive rejected")
	}
	if db.Exec("UPDATE media_workflow.execution_capabilities SET status='published' WHERE capability_key='jimeng.i2i.first_frame_prompt' AND version=2").Error == nil {
		t.Fatal("archived version revived")
	}
	if _, err := registry.Get(ctx, "jimeng.i2i.first_frame_prompt", 2); err == nil {
		t.Fatal("archived capability exposed as publishable")
	}
	down, err := os.ReadFile("../migrations/000023_provider_capability_contracts.down.sql")
	if err != nil {
		t.Fatal("down migration unavailable")
	}
	connection, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal("downgrade check connection unavailable")
	}
	_, downErr := connection.ExecContext(ctx, string(down))
	_, _ = connection.ExecContext(ctx, "ROLLBACK")
	_ = connection.Close()
	if downErr == nil {
		t.Fatal("downgrade erased provider contracts")
	}
	if c, err := registry.Get(ctx, "doubao.i2v.first_frame_prompt", 1); err != nil || c.ValidateContract() != nil {
		t.Fatal("failed downgrade damaged registry")
	}
}

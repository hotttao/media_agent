//go:build e2e

// Package e2e verifies migration and transaction behavior against real PostgreSQL.
package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_agent_gateway/biz/dal"
	repo "media_agent/media_agent_gateway/biz/dal/repo/agent_session"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
	sharedclient "media_agent/media_agent_gateway/biz/shared/client"
)

func TestCompletedRuntimeItemIsPersistedAsEventAndFullMessage(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("media_agent_gateway_e2e"),
		tcpostgres.WithUsername("gateway"),
		tcpostgres.WithPassword("gateway"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		t.Skipf("Docker/PostgreSQL unavailable: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if err := dal.Migrate(migrationsDir(t), &configpb.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn, MigrateOnStart: true}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })

	store := repo.New(db)
	sessions := service.New(store, dal.NewTxRunner(db), sharedclient.MockControl{})
	session, err := sessions.DefaultSession(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	_, turn, err := sessions.StartTurn(ctx, session.ID, "user-1", "request-1", "hello")
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"type":"item.completed","session_id":"runtime-1","item":{"item_id":"item-1","kind":"message","role":"assistant","status":"completed","content":[{"type":"text","text":"complete answer"}]}}`)
	if _, err := sessions.PersistEvent(ctx, session.ID, turn.ID, raw); err != nil {
		t.Fatal(err)
	}
	if err := sessions.FinishTurn(ctx, turn.ID, "completed"); err != nil {
		t.Fatal(err)
	}

	messages, err := sessions.Messages(ctx, session.ID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages=%d, want user + assistant", len(messages))
	}
	if messages[1].Role != "assistant" || !json.Valid(messages[1].Content) {
		t.Fatalf("assistant message not stored fully: %+v", messages[1])
	}
	var eventCount int
	if err := db.Raw("SELECT count(*) FROM media_agent_gateway.universal_events WHERE turn_id = ?", turn.ID).Scan(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("events=%d, want 1", eventCount)
	}
	var status string
	if err := db.Raw("SELECT status FROM media_agent_gateway.turns WHERE id = ?", turn.ID).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("turn status=%s", status)
	}
}

func migrationsDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		if filepath.Dir(dir) == dir {
			t.Fatalf("migrations not found from %s", wd)
		}
	}
}

package mcp_catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"media_agent/hertz_infra/messagebus"
	"media_agent/media_agent_gateway/biz/workflow"
	"os"
	"sync"
	"testing"
	"time"

	"media_agent/hertz_gen/config"
	"media_agent/media_agent_gateway/biz/dal"
	repo "media_agent/media_agent_gateway/biz/dal/repo/mcp_catalog"
	d "media_agent/media_agent_gateway/biz/domain/mcp_catalog"
	svc "media_agent/media_agent_gateway/biz/service/mcp_catalog"
)

func TestCatalogTransactionVersionsAndConcurrentCAS(t *testing.T) {
	dsn := os.Getenv("MEDIA_GATEWAY_OWNERSHIP_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	cfg := &config.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn, MigrateOnStart: true}
	if e := dal.Migrate("../../../../migrations", cfg); e != nil {
		t.Fatal(e)
	}
	db, e := dal.NewDB(cfg)
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	defer sql.Close()
	ctx := context.Background()
	store := repo.New(db)
	service := svc.New(store, dal.NewTxRunner(db), []string{"backend.test:9000"})
	definition := d.Definition{ID: "integration", ServiceName: "tools", Endpoint: "http://backend.test:9000/mcp", Transport: "streamable_http", AuthenticationType: "oauth", Status: "enabled"}
	defer db.Exec("DELETE FROM media_agent_gateway.mcp_catalog_events WHERE definition_id='integration'")
	defer db.Exec("DELETE FROM media_agent_gateway.mcp_definition_versions WHERE id='integration'")
	defer db.Exec("DELETE FROM media_agent_gateway.mcp_definitions WHERE id='integration'")
	if _, e = service.Put(ctx, definition, 0, false); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("non-admin registered", e)
	}
	invalid := definition
	invalid.Endpoint = "http://unapproved.test/mcp"
	if _, e = service.Put(ctx, invalid, 0, true); !errors.Is(e, d.ErrInvalid) {
		t.Fatal("unapproved host registered", e)
	}
	first, e := service.Put(ctx, definition, 0, true)
	if e != nil || first.Version != 1 {
		t.Fatal(first, e)
	}
	var wg sync.WaitGroup
	results := make([]error, 2)
	for n := range results {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			value := definition
			value.Status = "disabled"
			_, results[n] = service.Put(ctx, value, 1, true)
		}(n)
	}
	wg.Wait()
	if (results[0] == nil) == (results[1] == nil) {
		t.Fatalf("expected one CAS winner: %v", results)
	}
	historical, e := service.Version(ctx, "integration", 1)
	if e != nil || historical.Status != "enabled" {
		t.Fatal("history changed", historical, e)
	}
	current, e := service.Get(ctx, "integration")
	if e != nil || current.Version != 2 || current.Status != "disabled" {
		t.Fatal(current, e)
	}
	retired := definition
	retired.Status = "retired"
	if _, e = service.Put(ctx, retired, 2, true); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Put(ctx, definition, 3, true); !errors.Is(e, d.ErrConflict) {
		t.Fatal("retired definition revived", e)
	}
	var count int64
	db.Table("media_agent_gateway.mcp_catalog_events").Where("definition_id=?", definition.ID).Count(&count)
	if count != 3 {
		t.Fatalf("event count=%d", count)
	}
	events, e := service.PendingEvents(ctx, 100)
	if e != nil {
		t.Fatal(e)
	}
	if len(events) < 3 {
		t.Fatal("missing committed catalog events")
	}
	for _, event := range events {
		if event.DefinitionID == definition.ID {
			if e = service.CompleteEvent(ctx, event.ID); e != nil {
				t.Fatal(e)
			}
		}
	}
}

func TestCatalogCommittedOutboxPublishesThroughRabbit(t *testing.T) {
	dsn := os.Getenv("MEDIA_GATEWAY_OWNERSHIP_TEST_DSN")
	broker := os.Getenv("MEDIA_MCP_CATALOG_TEST_RABBITMQ_URL")
	if dsn == "" || broker == "" {
		t.Skip("isolated PostgreSQL and catalog Broker required")
	}
	cfg := &config.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn}
	db, e := dal.NewDB(cfg)
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	defer sql.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	exchange := "test.mcp.catalog." + uuid.NewString()
	consumer, e := messagebus.OpenTopicConsumer(ctx, broker, exchange)
	if e != nil {
		t.Fatal(e)
	}
	defer consumer.Close()
	if e = consumer.Bind("mcp.catalog.changed"); e != nil {
		t.Fatal(e)
	}
	publisher, e := messagebus.NewTopicPublisher(broker, exchange)
	if e != nil {
		t.Fatal(e)
	}
	defer publisher.Close()
	store := repo.New(db)
	service := svc.New(store, dal.NewTxRunner(db), []string{"backend.test"})
	id := "relay"
	defer db.Exec("DELETE FROM media_agent_gateway.mcp_catalog_events WHERE definition_id=?", id)
	defer db.Exec("DELETE FROM media_agent_gateway.mcp_definition_versions WHERE id=?", id)
	defer db.Exec("DELETE FROM media_agent_gateway.mcp_definitions WHERE id=?", id)
	_, e = service.Put(ctx, d.Definition{ID: id, ServiceName: id, Endpoint: "http://backend.test/mcp", Transport: "streamable_http", AuthenticationType: "none", Status: "enabled"}, 0, true)
	if e != nil {
		t.Fatal(e)
	}
	go workflow.RelayMCPCatalog(ctx, service, publisher)
	var received map[string]any
	if e = consumer.Receive(ctx, func(raw []byte) { json.Unmarshal(raw, &received) }); e != nil {
		t.Fatal(e)
	}
	if received["definition_id"] != id || received["definition_version"] != float64(1) || received["event_type"] != "mcp.catalog.changed" || received["event_id"] == nil {
		t.Fatalf("wrong catalog event: %+v", received)
	}
	for {
		var delivered int64
		db.Table("media_agent_gateway.mcp_catalog_events").Where("definition_id=? AND delivered_at IS NOT NULL", id).Count(&delivered)
		if delivered == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("publisher confirmed but outbox remained pending")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

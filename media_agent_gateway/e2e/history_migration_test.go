//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_agent_gateway/biz/dal"
	"media_agent/media_agent_gateway/biz/migration/agenthistory"
)

func TestLegacyHistoryMigratesAcrossPostgreSQLDatabasesBeforeCleanup(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("gateway"), tcpostgres.WithUsername("gateway"), tcpostgres.WithPassword("gateway"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	if err != nil {
		t.Skipf("Docker/PostgreSQL unavailable: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	gatewayDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := sql.Open("pgx", gatewayDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	if _, err = gateway.ExecContext(ctx, `CREATE DATABASE agents`); err != nil {
		t.Fatal(err)
	}
	agentsDSN := strings.Replace(gatewayDSN, "/gateway?", "/agents?", 1)
	agents, err := sql.Open("pgx", agentsDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer agents.Close()
	if err := dal.Migrate(migrationsDir(t), &configpb.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: gatewayDSN, MigrateOnStart: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = agents.ExecContext(ctx, agentSchema); err != nil {
		t.Fatal(err)
	}
	if _, err = gateway.ExecContext(ctx, legacyFixture); err != nil {
		t.Fatal(err)
	}
	migrator := agenthistory.Migrator{Source: gateway, Target: agents}
	if err := migrator.Import(ctx); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Import(ctx); err != nil {
		t.Fatalf("import is not idempotent: %v", err)
	}
	if err := migrator.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	var turns, messages, events int
	if err := agents.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM media_agents.turns),(SELECT count(*) FROM media_agents.messages),(SELECT count(*) FROM media_agents.universal_events)`).Scan(&turns, &messages, &events); err != nil {
		t.Fatal(err)
	}
	if turns != 1 || messages != 2 || events != 2 {
		t.Fatalf("target counts=%d/%d/%d", turns, messages, events)
	}
	var nextTurn, nextMessage, nextEvent int64
	if err := agents.QueryRowContext(ctx, `SELECT next_turn_sequence,next_message_sequence,next_event_sequence FROM media_agents.runtime_sessions WHERE session_id='11111111-1111-1111-1111-111111111111'`).Scan(&nextTurn, &nextMessage, &nextEvent); err != nil {
		t.Fatal(err)
	}
	if nextTurn != 2 || nextMessage != 3 || nextEvent != 3 {
		t.Fatalf("next sequences=%d/%d/%d", nextTurn, nextMessage, nextEvent)
	}
	if err := migrator.Cleanup(ctx, false); !errors.Is(err, agenthistory.ErrUnsafeCleanup) {
		t.Fatalf("unconfirmed cleanup error=%v", err)
	}
	if err := migrator.Cleanup(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, err = gateway.ExecContext(ctx, `SELECT 1 FROM media_agent_gateway.turns`); err == nil {
		t.Fatal("legacy tables still exist")
	}
}

func migrationsDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(wd), "migrations")
}

const agentSchema = `CREATE SCHEMA media_agents;
CREATE TABLE media_agents.runtime_sessions(session_id varchar(64) PRIMARY KEY,context_version bigint NOT NULL,context_ref varchar(512) NOT NULL,status varchar(16) NOT NULL,chat_status varchar(16) NOT NULL,active_turn_id varchar(64),next_turn_sequence bigint NOT NULL,next_event_sequence bigint NOT NULL,next_message_sequence bigint NOT NULL,runtime_session_id varchar(128),created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL);
CREATE TABLE media_agents.turns(id varchar(64) PRIMARY KEY,session_id varchar(64) NOT NULL REFERENCES media_agents.runtime_sessions(session_id),idempotency_key varchar(128) NOT NULL,enqueue_sequence bigint NOT NULL,status varchar(16) NOT NULL,input jsonb NOT NULL,context_version bigint NOT NULL,created_at timestamptz NOT NULL,started_at timestamptz,finished_at timestamptz,error_class varchar(64),UNIQUE(session_id,idempotency_key),UNIQUE(session_id,enqueue_sequence));
CREATE TABLE media_agents.messages(id varchar(64) PRIMARY KEY,session_id varchar(64) NOT NULL REFERENCES media_agents.runtime_sessions(session_id),turn_id varchar(64) NOT NULL REFERENCES media_agents.turns(id),role varchar(16) NOT NULL,content_ref jsonb NOT NULL,sequence bigint NOT NULL,created_at timestamptz NOT NULL,UNIQUE(session_id,sequence));
CREATE TABLE media_agents.universal_events(id varchar(64) PRIMARY KEY,session_id varchar(64) NOT NULL REFERENCES media_agents.runtime_sessions(session_id),turn_id varchar(64) REFERENCES media_agents.turns(id),sequence bigint NOT NULL,type varchar(64) NOT NULL,payload jsonb NOT NULL,created_at timestamptz NOT NULL,UNIQUE(session_id,sequence));`

const legacyFixture = `INSERT INTO media_agent_gateway.agent_sessions(id,user_id,workspace_id,agent_id,agent_type,title,context_version,status,chat_status,created_at,updated_at) VALUES('11111111-1111-1111-1111-111111111111','user','workspace','codex','codex','legacy',3,'active','human',now(),now());
INSERT INTO media_agent_gateway.turns(id,session_id,idempotency_key,status,started_at,finished_at) VALUES('22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111','legacy-key','completed',now(),now());
INSERT INTO media_agent_gateway.messages(id,session_id,turn_id,role,content,sequence) VALUES
('33333333-3333-3333-3333-333333333331','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','user','{"message":"hello"}',1),
('33333333-3333-3333-3333-333333333332','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','assistant','[{"type":"text","text":"hi"}]',2);
INSERT INTO media_agent_gateway.universal_events(id,session_id,turn_id,sequence,type,payload) VALUES
('44444444-4444-4444-4444-444444444441','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222',1,'item.completed','{"type":"item.completed"}'),
('44444444-4444-4444-4444-444444444442','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222',2,'turn.completed','{"type":"turn.completed"}');`

package agent_session

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	config "media_agent/hertz_gen/config"
	"media_agent/media_agent_gateway/biz/dal"
	d "media_agent/media_agent_gateway/biz/domain/agent_session"
)

func TestTwoGatewayConnectionsSerializeBindingsAndCredentials(t *testing.T) {
	dsn := os.Getenv("MEDIA_GATEWAY_OWNERSHIP_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	cfg := &config.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn, MigrateOnStart: true}
	if err := dal.Migrate("../../../../migrations", cfg); err != nil {
		t.Fatal(err)
	}
	first, err := dal.NewDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := dal.NewDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	firstSQL, _ := first.DB()
	defer firstSQL.Close()
	secondSQL, _ := second.DB()
	defer secondSQL.Close()
	a, b := New(first), New(second)
	ctx := context.Background()
	id := uuid.NewString()
	now := time.Now().UTC()
	session := &d.Session{ID: id, UserID: "owner", WorkspaceID: "workspace", AgentID: "codex", AgentType: "codex", AgentConfigID: "config", Status: d.StatusActive, ContextVersion: 7, BindingPhase: "unbound", ExecutionEpoch: 1, ExpectedExecutionEpoch: 1, CreatedAt: now, UpdatedAt: now}
	if err := a.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	defer first.Exec("DELETE FROM media_agent_gateway.agent_sessions WHERE id=?", id)
	assigned, err := a.AssignBinding(ctx, id, 0, d.Binding{EnvironmentID: "env", ReplicaID: "0", IncarnationID: "a", ExecutionEpoch: 1})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	repos := []*Repository{a, b}
	for i, r := range repos {
		wg.Add(1)
		go func(i int, r *Repository) {
			defer wg.Done()
			_, errs[i] = r.BeginTransfer(ctx, id, assigned.BindingVersion, d.Binding{EnvironmentID: "env", ReplicaID: string(rune('1' + i)), IncarnationID: "target", ExecutionEpoch: 1}, "transfer-"+string(rune('1'+i)))
		}(i, r)
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("expected one CAS winner: %v", errs)
	}
	loser := errs[0]
	if loser == nil {
		loser = errs[1]
	}
	if !errors.Is(loser, d.ErrBindingConflict) {
		t.Fatal(loser)
	}
	intent, err := b.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	actual := d.Binding{EnvironmentID: "env", ReplicaID: intent.TargetReplicaID, IncarnationID: "target", ExecutionEpoch: 2}
	completed, err := a.CompleteTransfer(ctx, id, intent.BindingVersion, intent.TransferID, actual)
	if err != nil || completed.ExecutionEpoch != 2 || completed.ContextVersion != 7 {
		t.Fatalf("completion: %+v %v", completed, err)
	}
	if _, err = b.CompleteTransfer(ctx, id, intent.BindingVersion, intent.TransferID, actual); !errors.Is(err, d.ErrBindingConflict) {
		t.Fatalf("stale completion: %v", err)
	}
	credentials := make([]*d.Session, 2)
	for i, r := range repos {
		wg.Add(1)
		go func(i int, r *Repository) {
			defer wg.Done()
			credentials[i], errs[i] = r.EnsureCredential(ctx, id, "hash-"+string(rune('a'+i)), "key")
		}(i, r)
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || credentials[0].TokenHash != credentials[1].TokenHash {
		t.Fatalf("credential CAS: %+v %+v %v", credentials[0], credentials[1], errs)
	}

}

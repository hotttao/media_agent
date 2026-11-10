package e2e

import (
	"context"
	"errors"
	"github.com/google/uuid"
	cfg "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/authn/appcredential"
	"media_agent/media_control/biz/dal"
	connectionrepo "media_agent/media_control/biz/dal/repo/model_connection"
	repo "media_agent/media_control/biz/dal/repo/workspace"
	config "media_agent/media_control/biz/domain/agent_configuration"
	connectiondomain "media_agent/media_control/biz/domain/model_connection"
	d "media_agent/media_control/biz/domain/workspace"
	connectionservice "media_agent/media_control/biz/service/model_connection"
	service "media_agent/media_control/biz/service/workspace"
	"media_agent/media_control/biz/shared/secret"
	flow "media_agent/media_control/biz/workflow/application_agent"
	"os"
	"sync"
	"testing"
)

type teamAccess struct{}

func (teamAccess) ListTeams(context.Context, string) ([]string, error) { return []string{"team"}, nil }
func (teamAccess) Authorize(context.Context, string, string, string) (d.TeamAuthorization, error) {
	return d.TeamAuthorization{Allowed: true, OrganizationID: "org", TeamID: "team"}, nil
}
func TestApplicationAgentConcurrentBindingAndIsolation(t *testing.T) {
	dsn := os.Getenv("MEDIA_CONTROL_APPLICATION_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	config := &cfg.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn, MigrateOnStart: true}
	if e := dal.Migrate("../migrations", config); e != nil {
		t.Fatal(e)
	}
	db, e := dal.NewDB(config)
	if e != nil {
		t.Fatal(e)
	}
	sql, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer sql.Close()
	r := repo.New(db)
	tx := dal.NewTxRunner(db)
	connections := connectionservice.New(connectionrepo.New(db), secret.New("isolated-test-only"))
	f := &flow.Flow{Workspaces: service.New(r, tx, secret.New("isolated-test-only"), teamAccess{}).WithModelConnections(connections), Bindings: service.NewApplicationBindings(r), Tx: tx}
	p := appcredential.Principal{ApplicationID: uuid.NewString(), UserID: uuid.NewString(), TeamID: "team", OrganizationID: "org"}
	cmd := service.CreateCommand{Name: "Assistant", AgentType: "goose"}
	ctx := context.Background()
	defer db.Exec("DELETE FROM workspaces WHERE owner_user_id=?", p.UserID)
	defer db.Exec("DELETE FROM application_agent_bindings WHERE user_id=?", p.UserID)
	type result struct {
		id      string
		created bool
		e       error
	}
	results := make(chan result, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			v, c, e := f.Ensure(ctx, p, "assistant", cmd)
			id := ""
			if v != nil {
				id = v.Workspace.ID
			}
			results <- result{id, c, e}
		})
	}
	wg.Wait()
	close(results)
	id := ""
	created := 0
	for r := range results {
		if r.e != nil {
			t.Fatal(r.e)
		}
		if id == "" {
			id = r.id
		}
		if r.id != id {
			t.Fatal("concurrent ensure created duplicate")
		}
		if r.created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created count %d", created)
	}
	cmd.Name = "Different"
	if _, _, e = f.Ensure(ctx, p, "assistant", cmd); !errors.Is(e, d.ErrConflict) {
		t.Fatalf("configuration conflict: %v", e)
	}
	if _, e = f.ByWorkspace(ctx, p, id); e != nil {
		t.Fatal(e)
	}
	other := p
	other.ApplicationID = "other"
	if _, e = f.Get(ctx, other, "assistant"); !errors.Is(e, d.ErrNotFound) {
		t.Fatalf("cross-app key: %v", e)
	}
	if _, e = f.ByWorkspace(ctx, other, id); !errors.Is(e, d.ErrNotFound) {
		t.Fatalf("cross-app workspace: %v", e)
	}
	other = p
	other.UserID = "other"
	if _, e = f.ByWorkspace(ctx, other, id); !errors.Is(e, d.ErrNotFound) {
		t.Fatalf("cross-user workspace: %v", e)
	}
	other = p
	other.TeamID = "other"
	if _, e = f.ByWorkspace(ctx, other, id); !errors.Is(e, d.ErrNotFound) {
		t.Fatalf("cross-team workspace: %v", e)
	}
	f.Metadata = failedMetadata{}
	cmd.Name = "Assistant"
	if _, _, e = f.Ensure(ctx, p, "rollback", cmd); e == nil {
		t.Fatal("metadata failure was ignored")
	}
	if _, e = f.Get(ctx, p, "rollback"); !errors.Is(e, d.ErrNotFound) {
		t.Fatalf("failed creation retained binding: %v", e)
	}
	var count int64
	db.Table("workspaces").Where("owner_user_id=?", p.UserID).Count(&count)
	if count != 1 {
		t.Fatalf("unexpected workspace count %d", count)
	}

	f.Metadata = nil
	raw := `{"tokens":{"access_token":"test-access","refresh_token":"test-refresh"}}`
	connection, e := connections.Create(ctx, p.UserID, "Codex", raw)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Exec("DELETE FROM model_connections WHERE id=?", connection.ID)
	codex := service.CreateCommand{Name: "Codex", AgentType: "codex", ModelConnectionID: connection.ID}
	bound, _, e := f.Ensure(ctx, p, "codex", codex)
	if e != nil || bound.ModelConnectionID != connection.ID {
		t.Fatalf("personal connection binding: %v", e)
	}
	other = p
	other.UserID = "other"
	if _, _, e = f.Ensure(ctx, other, "foreign", codex); !errors.Is(e, connectiondomain.ErrNotFound) {
		t.Fatalf("foreign connection accepted: %v", e)
	}
	invalid := codex
	invalid.APIKey = "test-key"
	if _, _, e = f.Ensure(ctx, p, "mixed", invalid); !errors.Is(e, flow.ErrInvalid) {
		t.Fatalf("mixed credentials: %v", e)
	}
	invalid = codex
	invalid.AgentType = "goose"
	if _, _, e = f.Ensure(ctx, p, "wrong-runtime", invalid); !errors.Is(e, flow.ErrInvalid) {
		t.Fatalf("wrong runtime: %v", e)
	}
	replacement, e := connections.Create(ctx, p.UserID, "Other", raw)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Exec("DELETE FROM model_connections WHERE id=?", replacement.ID)
	invalid = codex
	invalid.ModelConnectionID = replacement.ID
	if _, _, e = f.Ensure(ctx, p, "codex", invalid); !errors.Is(e, d.ErrConflict) {
		t.Fatalf("changed connection identity not rejected: %v", e)
	}
	if _, e = connections.Update(ctx, connection.ID, p.UserID, "", "", 1, true); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.Ensure(ctx, p, "codex", codex); !errors.Is(e, connectiondomain.ErrRevoked) {
		t.Fatalf("revoked connection accepted: %v", e)
	}
}

type failedMetadata struct{}

func (failedMetadata) ConfirmInTransaction(context.Context, config.Snapshot, string, string, string, int64) (*config.Release, error) {
	return nil, errors.New("injected metadata failure")
}

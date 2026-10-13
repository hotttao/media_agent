package runtime_environment_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	cfg "media_agent/hertz_gen/config"
	"media_agent/media_control/biz/dal"
	repo "media_agent/media_control/biz/dal/repo/runtime_environment"
	domain "media_agent/media_control/biz/domain/runtime_environment"
	service "media_agent/media_control/biz/service/runtime_environment"
	"os"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWorkloadTargetCASAndIdempotentRequest(t *testing.T) {
	dsn := os.Getenv("MEDIA_CONTROL_WORKLOAD_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	config := &cfg.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn, MigrateOnStart: true}
	if e := dal.Migrate("../../../../migrations", config); e != nil {
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
	svc := service.New(r, dal.NewTxRunner(db), nil)
	ws := "p5-" + uuid.NewString()
	defer db.Exec("DELETE FROM agent_workload_requests WHERE workspace_id=?", ws)
	defer db.Exec("DELETE FROM agent_workload_targets WHERE workspace_id=?", ws)
	v := domain.WorkloadTarget{WorkspaceID: ws, AgentConfigID: "cfg", EnvironmentID: "env", OwnerUserID: "owner", Spec: []byte(`{"image":"approved"}`), Replicas: 2, DesiredReplicas: 2, WorkspaceVersion: 1}
	first, e := svc.SetWorkload(ctx, v, 0, "initial", "hash-initial")
	if e != nil {
		t.Fatal(e)
	}
	if first.Version != 1 {
		t.Fatal("wrong initial version")
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			request := v
			request.Replicas = int32(i)
			request.DesiredReplicas = int32(i)
			request.Spec = []byte(`{"image":"unapproved"}`)
			_, e := svc.SetWorkload(ctx, request, 1, []string{"scale-a", "scale-b"}[i], []string{"hash-a", "hash-b"}[i])
			if e == nil {
				winners.Add(1)
			} else if !errors.Is(e, domain.ErrConflict) {
				t.Errorf("unexpected concurrent failure: %v", e)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("CAS has %d winners", winners.Load())
	}
	current, e := r.Workload(ctx, ws, false)
	if e != nil || current.Version != 2 {
		t.Fatalf("authoritative target invalid: %+v %v", current, e)
	}
	var spec map[string]string
	if json.Unmarshal(current.Spec, &spec) != nil || spec["image"] != "approved" {
		t.Fatal("target replaced approved specification")
	}
	replay, e := svc.SetWorkload(ctx, v, 0, "initial", "hash-initial")
	if e != nil || replay.Version != 1 {
		t.Fatal("retry did not return original accepted target")
	}
	if _, e = svc.SetWorkload(ctx, v, 0, "initial", "different-hash"); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("idempotency key accepted different request")
	}
	invalid := v
	invalid.WorkspaceID = ws + "-invalid"
	invalid.WorkspaceVersion = 0
	if _, e = svc.SetWorkload(ctx, invalid, 0, "initial", "invalid"); e == nil {
		t.Fatal("invalid lifecycle version accepted")
	}
	rows, e := r.ListWorkloads(ctx, ws, 100)
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range rows {
		if row.WorkspaceID == ws {
			t.Fatal("pagination includes cursor")
		}
	}
}

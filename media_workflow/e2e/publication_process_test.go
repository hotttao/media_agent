//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/workflow"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/publication"
)

type publicationCrashEngine struct {
	publication.RegistrationEngine
	crash bool
}

func (e publicationCrashEngine) EnsureWorkflow(ctx context.Context, d compiler.Definition) error {
	if err := e.RegistrationEngine.EnsureWorkflow(ctx, d); err != nil {
		return err
	}
	if e.crash {
		os.Exit(77)
	}
	return nil
}

func TestPublicationDeliveryProcessHelper(t *testing.T) {
	mode := os.Getenv("MEDIA_E2E_PUBLICATION_CHILD_MODE")
	if mode == "" {
		t.Skip("isolated subprocess helper")
	}
	db, err := gorm.Open(pgdriver.Open(os.Getenv("MEDIA_E2E_PUBLICATION_CHILD_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("child isolated database unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	flow := &publication.Delivery{Intents: repo.NewCompilationRepo(db), Workflows: repo.NewRepo(db), Tx: dal.NewTxRunner(db),
		Engine: publicationCrashEngine{RegistrationEngine: conductor.NewClient(os.Getenv("MEDIA_E2E_PUBLICATION_CHILD_ENGINE")), crash: mode == "registration-exit"}}
	if flow.DispatchOne(ctx) != nil {
		t.Fatal("child publication recovery failed")
	}
	if mode != "recover" {
		t.Fatal("crash boundary was not reached")
	}
}

// Two actual processes with isolated PostgreSQL and unique Conductor metadata.
// Recovery proxy forbids all writes: an attempted resend fails the gate even
// when an adapter subsequently hides it by reading an already matching version.
func TestPublicationDeliveryRealProcessRestart(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	db := newDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	name := "publication_restart_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	taskName := name + "_task"
	t.Cleanup(func() {
		for _, path := range []string{"/api/metadata/workflow/" + name + "/1", "/api/metadata/taskdefs/" + taskName} {
			req, _ := http.NewRequest(http.MethodDelete, address+path, nil)
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				t.Error("technical cleanup failed")
				continue
			}
			resp.Body.Close()
			if resp.StatusCode >= 300 && resp.StatusCode != 404 {
				t.Error("technical cleanup rejected")
			}
		}
	})
	workflows, store := repo.NewRepo(db), repo.NewCompilationRepo(db)
	if workflows.Save(ctx, &domain.Workflow{ID: name, Name: name, Scope: "e2e", Version: 1, CreatedBy: "owner", Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate, OutputStepKey: "technical"}) != nil {
		t.Fatal("source seed failed")
	}
	d := compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: taskName, TaskReferenceName: "technical", Type: "SIMPLE"}}}
	definition, _ := compiler.CanonicalJSON(d)
	hash, _ := compiler.DefinitionHash(d)
	tasks, _ := json.Marshal([]conductor.TaskDefinition{{Name: taskName, RetryCount: 0, TimeoutSeconds: 60, ResponseTimeoutSeconds: 30, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutPolicy: "TIME_OUT_WF"}})
	c := &domain.Compilation{ID: "comp_" + name, WorkflowID: name, WorkflowVersion: 1, ConductorName: name, ConductorVersion: 1, IRJSON: []byte(`{"technical":true}`), DefinitionJSON: definition, DefinitionHash: hash, Status: "registering", CreatedAt: time.Now()}
	p := domain.Publication{ID: "pub_" + name, WorkflowID: name, WorkflowVersion: 1, CompilationID: c.ID, RequestedBy: "owner", DefinitionJSON: definition, DefinitionHash: hash, TaskDefinitionsJSON: tasks}
	p.PayloadDigest, _ = p.Digest()
	if dal.NewTxRunner(db).RunInTx(ctx, func(tx context.Context) error { _, _, err := store.EnqueuePublication(tx, c, p); return err }) != nil {
		t.Fatal("intent seed failed")
	}
	upstream, err := url.Parse(address)
	if err != nil {
		t.Fatal("invalid upstream")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var readOnly atomic.Bool
	var taskWrites, workflowWrites, forbidden atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/metadata/") || readOnly.Load() && r.Method != "GET" {
			forbidden.Add(1)
			w.WriteHeader(503)
			return
		}
		if r.Method == "POST" {
			taskWrites.Add(1)
		}
		if r.Method == "PUT" {
			workflowWrites.Add(1)
		}
		proxy.ServeHTTP(w, r)
	}))
	defer server.Close()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	run := func(mode string, expected int) {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestPublicationDeliveryProcessHelper$", "-test.timeout=25s")
		cmd.Env = append(os.Environ(), "MEDIA_E2E_PUBLICATION_CHILD_MODE="+mode, "MEDIA_E2E_PUBLICATION_CHILD_DSN="+db.Dialector.(*pgdriver.Dialector).DSN, "MEDIA_E2E_PUBLICATION_CHILD_ENGINE="+server.URL)
		err := cmd.Run()
		if ctx.Err() != nil {
			t.Fatal("child observation timed out")
		}
		if expected == 0 {
			if err != nil {
				t.Fatal("recovery child failed; output suppressed")
			}
			return
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != expected {
			t.Fatal("child did not exit after registration before local completion")
		}
	}
	run("registration-exit", 77)
	current, _ := store.GetPublication(ctx, name, 1)
	source, _ := workflows.Get(ctx, name)
	if current.Status != "pending" || current.ClaimToken == "" || string(source.Status) != "publishing" {
		t.Fatal("crashed registration incorrectly completed locally")
	}
	if db.Exec("UPDATE media_workflow.workflow_publications SET claimed_at=now()-interval '3 minutes' WHERE id=?", p.ID).Error != nil {
		t.Fatal("isolated lease expiration failed")
	}
	readOnly.Store(true)
	run("recover", 0)
	current, _ = store.GetPublication(ctx, name, 1)
	source, _ = workflows.Get(ctx, name)
	if current.Status != "registered" || source.Status != domain.StatusPublished || current.PayloadDigest != p.PayloadDigest || taskWrites.Load() != 1 || workflowWrites.Load() != 1 || forbidden.Load() != 0 {
		t.Fatal("process recovery changed intent or resent metadata")
	}
}

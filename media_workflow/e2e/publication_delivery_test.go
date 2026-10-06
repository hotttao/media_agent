//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/workflow"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/publication"
)

type publicationRollbackTx struct{ base dal.TxRunner }

func (t publicationRollbackTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return t.base.RunInTx(ctx, func(tx context.Context) error {
		if err := fn(tx); err != nil {
			return err
		}
		return errors.New("injected rollback after publication writes")
	})
}

// Real Conductor registration and PostgreSQL completion. Creates only unique
// technical metadata, no Workflow instance, model, Worker or user queue work.
// Reconstruction is a new dispatcher/repository, not an OS process restart.
func TestPublicationDeliveryRealConductorRecovery(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	db := newDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	baseTx := dal.NewTxRunner(db)
	workflows := repo.NewRepo(db)
	for _, mode := range []string{"registration_response_loss", "local_completion_rollback"} {
		t.Run(mode, func(t *testing.T) {
			name := "publication_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			taskName := name + "_task"
			t.Cleanup(func() {
				for _, path := range []string{"/api/metadata/workflow/" + name + "/1", "/api/metadata/taskdefs/" + taskName} {
					req, _ := http.NewRequest(http.MethodDelete, address+path, nil)
					resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
					if err != nil {
						t.Error("technical metadata cleanup failed")
						continue
					}
					resp.Body.Close()
					if resp.StatusCode >= 300 && resp.StatusCode != 404 {
						t.Error("technical metadata cleanup rejected")
					}
				}
			})
			if workflows.Save(ctx, &domain.Workflow{ID: name, Name: name, Scope: "e2e", Version: 1, Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate, CreatedBy: "owner", OutputStepKey: "technical"}) != nil {
				t.Fatal("technical source seed failed")
			}
			definition := compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: taskName, TaskReferenceName: "technical", Type: "SIMPLE"}}}
			encoded, _ := compiler.CanonicalJSON(definition)
			hash, _ := compiler.DefinitionHash(definition)
			tasks, _ := json.Marshal([]conductor.TaskDefinition{{Name: taskName, RetryCount: 0, TimeoutSeconds: 60, ResponseTimeoutSeconds: 30, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutPolicy: "TIME_OUT_WF"}})
			c := &domain.Compilation{ID: "comp_" + name, WorkflowID: name, WorkflowVersion: 1, ConductorName: name, ConductorVersion: 1, IRJSON: []byte(`{"technical":true}`), DefinitionJSON: encoded, DefinitionHash: hash, Status: "registering", CreatedAt: time.Now()}
			p := domain.Publication{ID: "pub_" + name, WorkflowID: name, WorkflowVersion: 1, CompilationID: c.ID, RequestedBy: "owner", DefinitionJSON: encoded, DefinitionHash: hash, TaskDefinitionsJSON: tasks}
			p.PayloadDigest, _ = p.Digest()
			store := repo.NewCompilationRepo(db)
			if err := baseTx.RunInTx(ctx, func(tx context.Context) error { _, _, err := store.EnqueuePublication(tx, c, p); return err }); err != nil {
				t.Fatal("technical intent seed failed")
			}
			upstream, err := url.Parse(address)
			if err != nil {
				t.Fatal("upstream configuration invalid")
			}
			proxy := httputil.NewSingleHostReverseProxy(upstream)
			var taskWrites, workflowWrites, forbidden atomic.Int32
			proxy.ModifyResponse = func(resp *http.Response) error {
				write := false
				if resp.Request.Method == "POST" && resp.Request.URL.Path == "/api/metadata/taskdefs" {
					taskWrites.Add(1)
					write = true
				}
				if resp.Request.Method == "PUT" && resp.Request.URL.Path == "/api/metadata/workflow" {
					workflowWrites.Add(1)
					write = true
				}
				if write && mode == "registration_response_loss" && resp.StatusCode < 300 {
					resp.Body.Close()
					resp.StatusCode = 503
					resp.Body = io.NopCloser(strings.NewReader("injected committed registration response loss"))
					resp.ContentLength = -1
					resp.Header.Del("Content-Length")
				}
				return nil
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasPrefix(r.URL.Path, "/api/metadata/") {
					forbidden.Add(1)
					w.WriteHeader(500)
					return
				}
				proxy.ServeHTTP(w, r)
			}))
			defer server.Close()
			flow := &publication.Delivery{Intents: store, Workflows: workflows, Engine: conductor.NewClient(server.URL), Tx: baseTx}
			if mode == "local_completion_rollback" {
				flow.Tx = publicationRollbackTx{base: baseTx}
			}
			err = flow.DispatchOne(ctx)
			if mode == "local_completion_rollback" {
				if err == nil {
					t.Fatal("completion failure ignored")
				}
				current, _ := store.GetPublication(ctx, name, 1)
				source, _ := workflows.Get(ctx, name)
				if current.Status != "pending" || string(source.Status) != "publishing" {
					t.Fatal("local completion writes escaped rollback")
				}
				if db.Exec("UPDATE media_workflow.workflow_publications SET claimed_at=now()-interval '3 minutes' WHERE id=?", p.ID).Error != nil {
					t.Fatal("isolated lease expiration failed")
				}
				// Recover with no Template/Capability ports and a new repository.
				flow = &publication.Delivery{Intents: repo.NewCompilationRepo(db), Workflows: repo.NewRepo(db), Engine: conductor.NewClient(server.URL), Tx: baseTx}
				err = flow.DispatchOne(ctx)
			}
			if err != nil || taskWrites.Load() != 1 || workflowWrites.Load() != 1 || forbidden.Load() != 0 {
				t.Fatal("registration recovery resent metadata or failed")
			}
			current, err := store.GetPublication(ctx, name, 1)
			source, sourceErr := workflows.Get(ctx, name)
			if err != nil || sourceErr != nil || current.Status != "registered" || current.PayloadDigest != p.PayloadDigest || source.Status != domain.StatusPublished {
				t.Fatal("recovered publication did not preserve original intent")
			}
			registered, err := store.GetRegistered(ctx, name, 1)
			if err != nil || registered.DefinitionHash != hash {
				t.Fatal("frozen compilation not registered")
			}
		})
	}
}

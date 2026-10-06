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
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	delivery "media_agent/media_workflow/biz/workflow/start_delivery"
)

// Real engine + isolated PostgreSQL. Only HTTP responses are fault-injected;
// no user Job, model Worker, credential or production database is changed.
func TestExecutionStartDeliveryCancellationRecovery(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	db := newDatabase(t)
	store := repo.NewJobRepo(db)
	runner := dal.NewTxRunner(db)
	for _, mode := range []string{"cancel_before_send", "lost_start_then_recover", "lost_start_then_cancel", "lost_termination"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			name := "start_delivery_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			api := conductor.NewClient(address)
			if err := api.RegisterWorkflow(ctx, compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: "hold", TaskReferenceName: "hold", Type: "HUMAN"}}}); err != nil {
				t.Fatal("isolated definition registration failed")
			}
			var committed atomic.Value
			t.Cleanup(func() {
				cleanupCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
				defer done()
				if value := committed.Load(); value != nil {
					_ = api.TerminateWorkflow(cleanupCtx, value.(string), "isolated delivery cleanup")
				}
				req, _ := http.NewRequestWithContext(cleanupCtx, http.MethodDelete, address+"/api/metadata/workflow/"+name+"/1", nil)
				resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
				if err != nil {
					t.Error("isolated definition cleanup failed")
					return
				}
				resp.Body.Close()
				if resp.StatusCode >= 300 {
					t.Error("isolated definition cleanup rejected")
				}
			})
			upstream, err := url.Parse(address)
			if err != nil {
				t.Fatal("invalid configured upstream")
			}
			proxy := httputil.NewSingleHostReverseProxy(upstream)
			var posts, terminations atomic.Int32
			var blockLookup atomic.Bool
			lose := func(resp *http.Response) {
				resp.Body.Close()
				resp.StatusCode = http.StatusServiceUnavailable
				resp.Body = io.NopCloser(strings.NewReader("injected lost response"))
				resp.ContentLength = int64(len("injected lost response"))
				resp.Header.Del("Content-Length")
			}
			proxy.ModifyResponse = func(resp *http.Response) error {
				if resp.Request.Method == http.MethodPost && resp.Request.URL.Path == "/api/workflow" {
					posts.Add(1)
					if resp.StatusCode >= 300 {
						return nil
					}
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						return err
					}
					committed.Store(strings.Trim(strings.TrimSpace(string(body)), `"`))
					resp.Body.Close()
					resp.Body = io.NopCloser(strings.NewReader(string(body)))
					if strings.HasPrefix(mode, "lost_start_") {
						blockLookup.Store(true)
						lose(resp)
					}
				}
				if resp.Request.Method == http.MethodDelete && strings.HasPrefix(resp.Request.URL.Path, "/api/workflow/") {
					terminations.Add(1)
					if mode == "lost_termination" && resp.StatusCode < 300 {
						lose(resp)
					}
				}
				return nil
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if blockLookup.Load() && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/correlated/") {
					w.WriteHeader(503)
					return
				}
				proxy.ServeHTTP(w, r)
			}))
			defer server.Close()
			jobID := "job_" + name
			j := &domain.Job{ID: jobID, Scope: "e2e", WorkflowID: name, WorkflowVersion: 1, Status: domain.JobPending, SubmittedBy: "isolated-owner"}
			input, _ := json.Marshal(map[string]any{"job_id": jobID, "inputs": map[string]any{}})
			command := domain.ExecutionStart{ID: "start_" + name, JobID: jobID, WorkflowName: name, WorkflowVersion: 1, CompilationHash: strings.Repeat("a", 64), CorrelationID: jobID, WorkflowInput: input, TaskDomains: []byte(`{}`)}
			command.PayloadDigest, _ = command.Digest()
			if err := runner.RunInTx(ctx, func(tx context.Context) error {
				if err := store.Save(tx, j, nil); err != nil {
					return err
				}
				return store.EnqueueStart(tx, command)
			}); err != nil {
				t.Fatal("isolated aggregate seed failed")
			}
			flow := &delivery.Flow{Starts: store, Jobs: store, Engine: &conductor.ExecutionEngine{Client: conductor.NewClient(server.URL)}}
			if mode != "cancel_before_send" {
				launchErr := flow.DispatchOne(ctx)
				if strings.HasPrefix(mode, "lost_start_") {
					if !errors.Is(launchErr, delivery.ErrTemporarilyUnavailable) {
						t.Fatal("unknown start was not retained")
					}
				} else if launchErr != nil {
					t.Fatal("real start was not bound")
				}
				if committed.Load() == nil || posts.Load() != 1 {
					t.Fatal("real start not committed exactly once")
				}
			}
			if mode != "lost_start_then_recover" {
				if handled, err := store.CancelInitialStart(ctx, jobID); err != nil || !handled {
					t.Fatal("local cancellation failed")
				}
			}
			blockLookup.Store(false)
			// Advance only this isolated command's backoff, never user timestamps.
			advance := func() {
				if db.Exec("UPDATE media_workflow.execution_starts SET next_attempt_at=now()-interval '1 second' WHERE id=?", command.ID).Error != nil {
					t.Fatal("isolated backoff advance failed")
				}
			}
			advance()
			if mode == "lost_start_then_recover" {
				flow = &delivery.Flow{Starts: store, Jobs: store, Engine: &conductor.ExecutionEngine{Client: conductor.NewClient(server.URL)}}
				if err := flow.DispatchOne(ctx); err != nil {
					t.Fatal("unknown start reconciliation failed")
				}
				job, err := store.Get(ctx, jobID)
				if err != nil || job.Status != domain.JobRunning || job.ConductorWorkflowID != committed.Load().(string) || posts.Load() != 1 || terminations.Load() != 0 {
					t.Fatal("recovery failed to bind exactly the committed execution")
				}
				if err := flow.DispatchOne(ctx); err != nil || posts.Load() != 1 {
					t.Fatal("bound start was redelivered")
				}
				return
			}
			cleanupErr := flow.DispatchOne(ctx)
			if mode == "lost_termination" {
				if !errors.Is(cleanupErr, delivery.ErrTemporarilyUnavailable) {
					t.Fatal("lost termination outcome not retained")
				}
				advance()
				// Recreate coordinator/client; recovery must use persistent facts.
				flow = &delivery.Flow{Starts: store, Jobs: store, Engine: &conductor.ExecutionEngine{Client: conductor.NewClient(server.URL)}}
				cleanupErr = flow.DispatchOne(ctx)
			}
			if cleanupErr != nil {
				t.Fatal("real cancellation did not converge")
			}
			var audit struct{ Status, ConductorWorkflowID string }
			if db.Table("media_workflow.execution_starts").Where("id=?", command.ID).Take(&audit).Error != nil || audit.Status != "cancelled" {
				t.Fatal("cancellation audit not completed")
			}
			job, err := store.Get(ctx, jobID)
			if err != nil || job.Status != domain.JobCancelled {
				t.Fatal("Job was revived")
			}
			if mode == "cancel_before_send" {
				if posts.Load() != 0 || terminations.Load() != 0 || audit.ConductorWorkflowID != "" {
					t.Fatal("unsent cancellation invoked engine")
				}
			} else {
				id := committed.Load().(string)
				execution, err := api.GetWorkflow(ctx, id)
				if err != nil || execution.Status != "TERMINATED" || audit.ConductorWorkflowID != id || posts.Load() != 1 || terminations.Load() != 1 {
					t.Fatal("recovery repeated side effects or lost original instance")
				}
			}
			if err := flow.DispatchOne(ctx); err != nil {
				t.Fatal("finished command was redelivered")
			}
		})
	}
}

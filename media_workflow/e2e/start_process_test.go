//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	delivery "media_agent/media_workflow/biz/workflow/start_delivery"
)

type startProcessEngine struct {
	*conductor.ExecutionEngine
	mode                         string
	forbidStart, forbidTerminate bool
}

func (e startProcessEngine) Start(ctx context.Context, name string, version int, correlation string, input map[string]any, domains map[string]string) (string, error) {
	if e.forbidStart {
		os.Exit(88) // Any attempted resend fails the parent's expected exit=0.
	}
	id, err := e.ExecutionEngine.Start(ctx, name, version, correlation, input, domains)
	if err == nil && id != "" && e.mode == "start-commit-exit" {
		os.Exit(77)
	}
	return id, err
}
func (e startProcessEngine) Terminate(ctx context.Context, id, reason string) error {
	if e.forbidTerminate {
		os.Exit(88)
	}
	if err := e.ExecutionEngine.Terminate(ctx, id, reason); err != nil {
		return err
	}
	if e.mode == "terminate-commit-exit" {
		os.Exit(77)
	}
	return nil
}

// Only entered by the parent with an isolated DSN. os.Exit deliberately skips
// defers, simulating a process dying while holding its durable delivery lease.
func TestExecutionStartProcessHelper(t *testing.T) {
	mode := os.Getenv("MEDIA_E2E_START_CHILD_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	db, err := gorm.Open(pgdriver.Open(os.Getenv("MEDIA_E2E_START_CHILD_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("isolated child database unavailable")
	}
	store := repo.NewJobRepo(db)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if mode == "claim-exit" || mode == "mark-exit" {
		command, err := store.ClaimStart(ctx)
		if err != nil || command == nil || command.ID != os.Getenv("MEDIA_E2E_START_CHILD_ID") {
			t.Fatal("child did not claim the expected command")
		}
		if mode == "mark-exit" {
			if err := store.MarkStartSent(ctx, *command); err != nil {
				t.Fatal("child sent mark failed")
			}
		}
		os.Exit(77)
	}
	engine := startProcessEngine{ExecutionEngine: &conductor.ExecutionEngine{Client: conductor.NewClient(os.Getenv("MEDIA_E2E_CONDUCTOR_URL"))}, mode: mode, forbidStart: os.Getenv("MEDIA_E2E_START_CHILD_FORBID_START") == "true", forbidTerminate: os.Getenv("MEDIA_E2E_START_CHILD_FORBID_TERMINATE") == "true"}
	flow := &delivery.Flow{Starts: store, Jobs: store, Engine: engine}
	err = flow.DispatchOne(ctx)
	if mode == "observe-unsent-uncertain" {
		if !errors.Is(err, delivery.ErrTemporarilyUnavailable) {
			t.Fatal("ambiguous unsent command was not retained")
		}
	} else if err != nil {
		t.Fatal("new process did not recover durable delivery")
	}
}

func TestExecutionStartRealProcessRestart(t *testing.T) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	db := newDatabase(t)
	store := repo.NewJobRepo(db)
	// Keep mark-exit last: it intentionally retains an unresolved command.
	for _, mode := range []string{"claim-exit", "start-commit-exit", "terminate-commit-exit", "mark-exit"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			name := "start_restart_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			api := conductor.NewClient(address)
			if err := api.RegisterWorkflow(ctx, compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: "hold", TaskReferenceName: "hold", Type: "HUMAN"}}}); err != nil {
				t.Fatal("isolated definition registration failed")
			}
			jobID := "job_" + name
			t.Cleanup(func() {
				cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
				defer done()
				if id, err := api.GetByCorrelation(cleanup, name, jobID); err == nil && id != "" {
					_ = api.TerminateWorkflow(cleanup, id, "isolated restart cleanup")
				}
				req, _ := http.NewRequestWithContext(cleanup, http.MethodDelete, address+"/api/metadata/workflow/"+name+"/1", nil)
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
			input, _ := json.Marshal(map[string]any{"job_id": jobID})
			command := domain.ExecutionStart{ID: "start_" + name, JobID: jobID, WorkflowName: name, WorkflowVersion: 1, CompilationHash: strings.Repeat("a", 64), CorrelationID: jobID, WorkflowInput: input, TaskDomains: []byte(`{}`)}
			command.PayloadDigest, _ = command.Digest()
			if err := dal.NewTxRunner(db).RunInTx(ctx, func(tx context.Context) error {
				if err := store.Save(tx, &domain.Job{ID: jobID, Scope: "e2e", WorkflowID: name, WorkflowVersion: 1, Status: domain.JobPending, SubmittedBy: "isolated-owner"}, nil); err != nil {
					return err
				}
				return store.EnqueueStart(tx, command)
			}); err != nil {
				t.Fatal("isolated start seed failed")
			}
			engine := &conductor.ExecutionEngine{Client: api}
			flow := &delivery.Flow{Starts: store, Jobs: store, Engine: engine}
			if mode == "terminate-commit-exit" {
				if err := flow.DispatchOne(ctx); err != nil {
					t.Fatal("cancel fixture did not launch")
				}
				if handled, err := store.CancelInitialStart(ctx, jobID); err != nil || !handled {
					t.Fatal("cancel fixture not persisted")
				}
			}
			run := func(childMode string, exit int, forbidStart, forbidTerminate bool) {
				t.Helper()
				binary, err := os.Executable()
				if err != nil {
					t.Fatal("test executable unavailable")
				}
				cmd := exec.CommandContext(ctx, binary, "-test.run=^TestExecutionStartProcessHelper$", "-test.timeout=20s")
				cmd.Env = append(os.Environ(), "MEDIA_E2E_START_CHILD_MODE="+childMode, "MEDIA_E2E_START_CHILD_DSN="+db.Dialector.(*pgdriver.Dialector).DSN, "MEDIA_E2E_START_CHILD_ID="+command.ID)
				if forbidStart {
					cmd.Env = append(cmd.Env, "MEDIA_E2E_START_CHILD_FORBID_START=true")
				}
				if forbidTerminate {
					cmd.Env = append(cmd.Env, "MEDIA_E2E_START_CHILD_FORBID_TERMINATE=true")
				}
				_, err = cmd.CombinedOutput() // Never print potentially private child diagnostics.
				if exit == 0 {
					if err != nil {
						t.Fatal("delivery subprocess failed")
					}
					return
				}
				var failed *exec.ExitError
				if !errors.As(err, &failed) || failed.ExitCode() != exit {
					t.Fatal("subprocess did not exit at expected crash boundary")
				}
			}
			run(mode, 77, false, false)
			var abandoned struct{ Status, ClaimToken string }
			if db.Table("media_workflow.execution_starts").Where("id=?", command.ID).Take(&abandoned).Error != nil || abandoned.ClaimToken == "" {
				t.Fatal("crash did not leave persisted lease")
			}
			if db.Exec("UPDATE media_workflow.execution_starts SET claimed_at=now()-interval '3 minutes',next_attempt_at=now()-interval '1 second' WHERE id=?", command.ID).Error != nil {
				t.Fatal("isolated lease expiry failed")
			}
			recovery := "recover"
			if mode == "mark-exit" {
				recovery = "observe-unsent-uncertain"
			}
			run(recovery, 0, mode != "claim-exit", mode == "terminate-commit-exit")
			job, err := store.Get(ctx, jobID)
			if err != nil {
				t.Fatal("recovered Job unavailable")
			}
			var saved struct {
				Status, ConductorWorkflowID, ClaimToken string
				DeliveryAttempts                        int
			}
			attempts := 2
			if mode == "terminate-commit-exit" {
				attempts = 3
			} // Initial launch, cleanup crash, cleanup recovery.
			if db.Table("media_workflow.execution_starts").Where("id=?", command.ID).Take(&saved).Error != nil || saved.DeliveryAttempts != attempts {
				t.Fatal("abandoned lease not reclaimed once")
			}
			id, err := api.GetByCorrelation(ctx, name, jobID)
			if err != nil {
				t.Fatal("recovery created ambiguous duplicate executions")
			}
			switch mode {
			case "mark-exit":
				if id != "" || job.Status != domain.JobPending || saved.Status != "uncertain" || saved.ClaimToken != "" {
					t.Fatal("unsent uncertain command was misrepresented as recovered")
				}
				t.Log("verified safety boundary: unsent uncertain remains unresolved, never automatically replayed")
			case "terminate-commit-exit":
				execution, err := api.GetWorkflow(ctx, id)
				if err != nil || execution.Status != "TERMINATED" || job.Status != domain.JobCancelled || saved.Status != "cancelled" || saved.ConductorWorkflowID != id {
					t.Fatal("committed cancellation not recovered")
				}
			default:
				if id == "" || job.Status != domain.JobRunning || job.ConductorWorkflowID != id || saved.Status != "started" || saved.ConductorWorkflowID != id {
					t.Fatal("new process did not bind original execution")
				}
			}
		})
	}
}

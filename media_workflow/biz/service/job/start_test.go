package job

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared/client"
)

type startTxKey struct{}
type startTxProbe struct{ calls int }

func (p *startTxProbe) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	p.calls++
	return fn(context.WithValue(ctx, startTxKey{}, true))
}

type startAcceptanceRepo struct {
	domain.ExecutionStartRepo
	jobs    *mockJobRepo
	command *domain.ExecutionStart
	err     error
}

func (r *startAcceptanceRepo) EnqueueStart(ctx context.Context, c domain.ExecutionStart) error {
	if ctx.Value(startTxKey{}) != true {
		return errors.New("transaction missing")
	}
	j, ok := r.jobs.jobs[c.JobID]
	if !ok || len(r.jobs.stepRuns[c.JobID]) != 1 || j.Status != domain.JobPending {
		return errors.New("aggregate missing")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	r.command = &c
	return r.err
}

type transactionalJobProbe struct{ *mockJobRepo }

func (p transactionalJobProbe) Save(ctx context.Context, j *domain.Job, steps []*domain.StepRun) error {
	if ctx.Value(startTxKey{}) != true {
		return errors.New("job save outside transaction")
	}
	return p.mockJobRepo.Save(ctx, j, steps)
}

func TestSubmitDurableStartAcceptsPendingWithoutEngineCall(t *testing.T) {
	for _, tc := range []struct {
		name                                             string
		invalidIR, missingTx, missingStarts, failEnqueue bool
	}{
		{name: "accepted"}, {name: "invalid IR", invalidIR: true}, {name: "missing transaction", missingTx: true}, {name: "missing start repository", missingStarts: true}, {name: "enqueue fails", failEnqueue: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newTestService(false)
			svc.jobRepo = transactionalJobProbe{repo}
			wf := svc.wfRepo.(*mockWorkflowLookup).wf
			wf.CreatedBy, wf.Status = "owner", wfdomain.StatusPublished
			a := &submitAuthority{workspace: nil, allowed: true}
			// Reuse the existing authoritative workspace fixture type.
			svc.historyWorkspaces = historyWorkspaceLookup{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "org"}}
			svc.resumeAccess = a
			engine := &mockWorkflowEngine{}
			svc.engine = engine
			compilation := &wfdomain.Compilation{ConductorName: "frozen", ConductorVersion: 3, DefinitionHash: strings.Repeat("a", 64), IRJSON: []byte(`{"schema_version":3,"steps":[]}`)}
			if tc.invalidIR {
				compilation.IRJSON = []byte(`broken`)
			}
			svc.compilations = &retryCompilationStub{item: compilation}
			tx := &startTxProbe{}
			svc.tx = tx
			if tc.missingTx {
				svc.tx = nil
			}
			starts := &startAcceptanceRepo{jobs: repo}
			if tc.failEnqueue {
				starts.err = errors.New("private database detail")
			}
			svc.ConfigureStarts(starts)
			if tc.missingStarts {
				svc.ConfigureStarts(nil)
			}
			j, err := svc.Submit(context.Background(), SubmitCommand{WorkflowID: "wf1", SubmittedBy: "owner", WorkspaceID: "workspace", Input: domain.JobInput{Materials: []domain.MaterialData{{Name: "scene", URL: "https://example.test/media/materials/org/scene.png"}}}})
			if engine.correlationID != "" {
				t.Fatal("submission called upstream")
			}
			if tc.invalidIR || tc.missingTx || tc.missingStarts || tc.failEnqueue {
				if err == nil || j != nil {
					t.Fatal("invalid submission accepted")
				}
				if strings.Contains(err.Error(), "private") {
					t.Fatal("sensitive error returned")
				}
				if !tc.failEnqueue && len(repo.jobs) != 0 {
					t.Fatal("invalid payload persisted before validation")
				}
				return
			}
			if err != nil || j.Status != domain.JobPending || j.ConductorWorkflowID != "" || j.Origin != "native" || j.CompilationHash != compilation.DefinitionHash || starts.command == nil || tx.calls != 1 {
				t.Fatalf("pending acceptance invalid: %v", err)
			}
			if starts.command.WorkflowVersion != 3 || starts.command.CorrelationID != j.ID || !strings.Contains(string(starts.command.WorkflowInput), "object_path:materials/org/scene.png") {
				t.Fatal("start did not freeze delivery input")
			}
		})
	}
}

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

type submissionStore struct {
	receipt              *domain.Submission
	reads, writes, locks int
	missOnce             bool
}

func (r *submissionStore) GetSubmission(ctx context.Context, key domain.SubmissionKey, lock bool) (*domain.Submission, error) {
	r.reads++
	if lock {
		if ctx.Value(startTxKey{}) != true {
			return nil, errors.New("lock outside transaction")
		}
		r.locks++
	} else if r.missOnce {
		r.missOnce = false
		return nil, nil
	}
	if r.receipt == nil || r.receipt.SubmissionKey != key {
		return nil, nil
	}
	copy := *r.receipt
	return &copy, nil
}
func (r *submissionStore) SaveSubmission(ctx context.Context, receipt domain.Submission) error {
	if ctx.Value(startTxKey{}) != true || receipt.Validate() != nil {
		return errors.New("invalid receipt transaction")
	}
	r.writes++
	r.receipt = &receipt
	return nil
}

func submissionFixture() (*Service, *mockJobRepo, *startAcceptanceRepo, *submissionStore, *submitAuthority, SubmitCommand) {
	svc, jobs, _ := newTestService(false)
	svc.jobRepo = transactionalJobProbe{jobs}
	wf := svc.wfRepo.(*mockWorkflowLookup).wf
	wf.CreatedBy, wf.Status = "owner", wfdomain.StatusPublished
	a := &submitAuthority{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "org"}, allowed: true}
	svc.historyWorkspaces, svc.resumeAccess = a, a
	svc.compilations = &retryCompilationStub{item: &wfdomain.Compilation{ConductorName: "frozen", ConductorVersion: 3, DefinitionHash: strings.Repeat("a", 64), IRJSON: []byte(`{"schema_version":3,"steps":[]}`)}}
	svc.engine = &mockWorkflowEngine{}
	svc.tx = &startTxProbe{}
	starts := &startAcceptanceRepo{jobs: jobs}
	ledger := &submissionStore{}
	svc.ConfigureStarts(starts)
	svc.ConfigureSubmissions(ledger)
	cmd := SubmitCommand{WorkflowID: "wf1", SubmittedBy: "owner", WorkspaceID: "workspace", IdempotencyKey: "intent:one", Input: domain.JobInput{Materials: []domain.MaterialData{{Name: "scene", URL: "https://example.test/media/materials/org/scene.png?signature=old", Content: "scene"}}}}
	return svc, jobs, starts, ledger, a, cmd
}

func TestSubmitReceiptReplayPrecedesPublicationAndMediaRead(t *testing.T) {
	svc, jobs, starts, ledger, a, cmd := submissionFixture()
	var reads int
	svc.resumeURLVerifier = func(context.Context, string) error { reads++; return nil }
	j, err := svc.Submit(context.Background(), cmd)
	if err != nil || ledger.writes != 1 || ledger.locks != 1 || reads != 1 || starts.command == nil {
		t.Fatal("initial acceptance did not freeze complete aggregate", err)
	}
	j.Status = domain.JobSucceeded
	// The published definition is gone and the original media signature has
	// expired. Neither is required to return an already authorized receipt.
	svc.wfRepo = &mockWorkflowLookup{}
	svc.resumeURLVerifier = func(context.Context, string) error { t.Fatal("replay fetched media"); return nil }
	cmd.Input.Materials[0].URL = "https://example.test/media/materials/org/scene.png?signature=new"
	replayed, err := svc.Submit(context.Background(), cmd)
	if err != nil || replayed.ID != j.ID || replayed.Status != domain.JobSucceeded || len(jobs.jobs) != 1 || ledger.writes != 1 {
		t.Fatal("terminal stable-media replay failed", err)
	}
	cmd.Input.Materials[0].Content = "changed"
	if _, err := svc.Submit(context.Background(), cmd); !errors.Is(err, domain.ErrSubmissionConflict) {
		t.Fatal("same key changed semantic content")
	}
	cmd.Input.Materials[0].Content = "scene"
	cmd.WorkflowVersion = 1 // Requested 0 and explicitly requested 1 differ.
	if _, err := svc.Submit(context.Background(), cmd); !errors.Is(err, domain.ErrSubmissionConflict) {
		t.Fatal("version intent changed without conflict")
	}
	cmd.WorkflowVersion = 0
	a.allowed = false
	before := ledger.reads
	if _, err := svc.Submit(context.Background(), cmd); !errors.Is(err, ErrSubmitForbidden) || ledger.reads != before {
		t.Fatal("revoked permission reached receipt lookup")
	}
	if svc.engine.(*mockWorkflowEngine).correlationID != "" {
		t.Fatal("receipt acceptance launched synchronously")
	}
}

func TestSubmitConcurrentWinnerRecheckedBeforeAnyAggregateSave(t *testing.T) {
	svc, jobs, starts, ledger, _, cmd := submissionFixture()
	j, err := svc.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	originalStart := starts.command
	// The preflight lookup observed no committed winner, but another request
	// committed before the caller acquired its transaction-scoped key lock.
	ledger.missOnce = true
	replayed, err := svc.Submit(context.Background(), cmd)
	if err != nil || replayed.ID != j.ID || ledger.writes != 1 || ledger.locks != 2 || len(jobs.jobs) != 1 || starts.command != originalStart {
		t.Fatal("winner recheck created another aggregate", err)
	}
	ledger.missOnce = true
	cmd.Input.Materials[0].Content = "racing different input"
	if _, err := svc.Submit(context.Background(), cmd); !errors.Is(err, domain.ErrSubmissionConflict) || len(jobs.jobs) != 1 || ledger.writes != 1 {
		t.Fatal("conflicting concurrent request persisted a Job", err)
	}
}

func TestSubmitReceiptCannotExposeForeignOrCorruptAcceptedJob(t *testing.T) {
	for _, change := range []func(*domain.Job){
		func(j *domain.Job) { j.SubmittedBy = "other" },
		func(j *domain.Job) { j.OrganizationID = "other" },
		func(j *domain.Job) { j.WorkspaceID = "other" },
		func(j *domain.Job) { j.Origin = "legacy_import" },
	} {
		svc, _, _, _, _, cmd := submissionFixture()
		j, err := svc.Submit(context.Background(), cmd)
		if err != nil {
			t.Fatal(err)
		}
		change(j)
		if _, err := svc.Submit(context.Background(), cmd); !errors.Is(err, ErrSubmitForbidden) {
			t.Fatal("receipt returned mismatched Job", err)
		}
	}
}

func TestSubmitVersionRaceReturnsConflictBeforeCreatingJob(t *testing.T) {
	svc, repo, _, _, _, _ := submissionFixture()
	_, err := svc.Submit(context.Background(), SubmitCommand{WorkflowID: "wf1", WorkflowVersion: 999, SubmittedBy: "owner", OrganizationID: "org", WorkspaceID: "workspace"})
	if !errors.Is(err, ErrSubmitVersionConflict) || len(repo.jobs) != 0 {
		t.Fatalf("version change did not reject submission: %v", err)
	}
}

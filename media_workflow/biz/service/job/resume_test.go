package job

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "media_agent/media_workflow/biz/domain/job"
	templatedomain "media_agent/media_workflow/biz/domain/template"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/executable"
)

type fakeResumeRepo struct {
	pending  *domain.ResumeStart
	started  string
	released string
	accepted *domain.ResumeStart
	input    domain.JobInput
	job      *domain.Job
}

func (r *fakeResumeRepo) BeginResume(_ context.Context, start domain.ResumeStart, input domain.JobInput) (bool, error) {
	r.accepted = &start
	r.input = input
	if r.job != nil {
		r.job.Status = domain.JobRunning
		r.job.Input = input
		r.job.ResumeGeneration = start.Generation
		r.job.ResumeIdempotencyKey = start.IdempotencyKey
	}
	return true, nil
}
func (r *fakeResumeRepo) ClaimPendingResume(context.Context) (*domain.ResumeStart, error) {
	start := r.pending
	r.pending = nil
	return start, nil
}
func (r *fakeResumeRepo) MarkResumeStarted(_ context.Context, _ domain.ResumeStart, executionID, _ string) error {
	r.started = executionID
	return nil
}
func (r *fakeResumeRepo) ReleaseResumeClaim(_ context.Context, _ string, reason string) error {
	r.released = reason
	return nil
}

type fakeResumeLauncher struct{ err error }

func (l fakeResumeLauncher) StartResume(context.Context, domain.ResumeStart) (string, string, error) {
	if l.err != nil {
		return "", "", l.err
	}
	return "conductor-1", "definition-hash", nil
}

type fakeResumeAccess struct{ allowed bool }

func (a fakeResumeAccess) CanUseWorkflow(context.Context, string, string) (bool, error) {
	return a.allowed, nil
}

type fakeResumeTemplates struct{}

func (fakeResumeTemplates) GetVersion(context.Context, string, int) (*wfdomain.TemplateSnapshot, error) {
	return &wfdomain.TemplateSnapshot{Inputs: []wfdomain.SlotSnapshot{{Name: "movement", ResourceType: "MATERIAL", AssetKind: "movement"}}}, nil
}

func TestResumeFreezesOnlyPendingVideoAndIsIdempotent(t *testing.T) {
	svc, repo, importCmd := historyImportFixture(t)
	job, err := svc.ImportIncomplete(context.Background(), importCmd)
	if err != nil {
		t.Fatal(err)
	}
	lookup := svc.historyAssets.(historyAssetLookup)
	for _, asset := range lookup.assets {
		asset.AccessURL = "http://media-storage.example/media/historical.png"
		asset.AccessExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	}
	keys := []string{"model_image", "style_image", "first_frame", "video"}
	ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: "wf", WorkflowVersion: 1, OutputStepKey: "video"}
	for i, key := range keys {
		step := executable.ExecutableStepIR{
			Key: key, Order: i + 1, Template: executable.TemplateIR{ID: "tpl-" + key, Version: 1, Body: "{{.movement.content}}"},
			Capability:      executable.CapabilityIR{Key: key, Version: 1, ConductorTaskDef: "cap_" + key, ExecutionTarget: "personal_worker"},
			OutputAssetKind: []string{"model_image", "style_image", "first_frame", "ai_video"}[i],
		}
		if i > 0 {
			step.DependsOn = []string{keys[i-1]}
		}
		if key == "video" {
			step.Context = map[string]executable.ValueRef{
				"movement":    {Source: executable.ValueWorkflowInput, Ref: "movement"},
				"first_frame": {Source: executable.ValueStepOutput, Ref: "first_frame"},
			}
			step.Invocation = templatedomain.InvocationMapping{"prompt": {Op: templatedomain.InvocationRenderTemplate}}
		}
		ir.Steps = append(ir.Steps, step)
	}
	encoded, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	svc.compilations = historyCompilationRepo{ir: encoded}
	svc.tplLookup = fakeResumeTemplates{}
	outbox := &fakeResumeRepo{job: repo.jobs[job.ID]}
	svc.ConfigureResume(outbox, fakeResumeAccess{allowed: true}, fakeResumeLauncher{}, "http://media-storage.example")
	svc.resumeURLVerifier = func(context.Context, string) error { return nil }
	cmd := ResumeCommand{JobID: job.ID, UserID: "owner", IdempotencyKey: "continue-once"}
	if _, err := svc.Resume(context.Background(), cmd); err == nil || !strings.Contains(err.Error(), "movement") {
		t.Fatalf("missing movement not rejected: %v", err)
	}
	if outbox.accepted != nil {
		t.Fatal("outbox written before input validation")
	}
	cmd.InputPatch.Materials = []domain.MaterialData{{Name: "movement", AssetKind: "movement", URL: "http://media-storage.example/media/materials/org/movement.mp4", Content: "walk"}}
	resumed, err := svc.Resume(context.Background(), cmd)
	if err != nil || resumed.Status != domain.JobRunning || outbox.accepted == nil || outbox.accepted.CorrelationID != "resume:"+job.ID+":1" {
		t.Fatalf("resume was not enqueued: job=%+v start=%+v err=%v", resumed, outbox.accepted, err)
	}
	if got := outbox.input.Materials[0].URL; got != "object_path:materials/org/movement.mp4" {
		t.Fatalf("resume persisted a URL instead of stable object path: %q", got)
	}
	if strings.Contains(string(outbox.accepted.WorkflowInputJSON), "media-storage.example") {
		t.Fatal("StartResume Outbox contains a transport URL")
	}
	var definition struct {
		Tasks []struct {
			Name string `json:"name"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(outbox.accepted.DefinitionJSON, &definition); err != nil || len(definition.Tasks) != 6 || definition.Tasks[0].Name != "media_restore_historical_step_v1" {
		t.Fatalf("historical prefix was not compiled to restore tasks: definition=%+v err=%v", definition, err)
	}
	firstStart := outbox.accepted
	if _, err := svc.Resume(context.Background(), cmd); err != nil || outbox.accepted != firstStart {
		t.Fatalf("same resume key started again: err=%v", err)
	}
	repo.jobs[job.ID].Status = domain.JobSucceeded
	if completed, err := svc.Resume(context.Background(), cmd); err != nil || completed.Status != domain.JobSucceeded || outbox.accepted != firstStart {
		t.Fatalf("same resume key after completion was not idempotent: job=%+v err=%v", completed, err)
	}
	cmd.IdempotencyKey = "different"
	if _, err := svc.Resume(context.Background(), cmd); !errors.Is(err, ErrResumeConflict) {
		t.Fatalf("different resume key was not rejected: %v", err)
	}
}

func TestVerifyResumeURLReachableUsesBoundedGetWithoutRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/available", http.StatusFound)
			return
		}
		if r.Header.Get("Range") != "bytes=0-0" || r.Method != http.MethodGet {
			t.Errorf("unexpected object probe: %s range=%q", r.Method, r.Header.Get("Range"))
		}
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer server.Close()
	if err := verifyResumeURLReachable(context.Background(), server.URL+"/available"); err != nil {
		t.Fatalf("readable object rejected: %v", err)
	}
	if err := verifyResumeURLReachable(context.Background(), server.URL+"/redirect"); err == nil {
		t.Fatal("redirected object accepted")
	}
}

func TestDispatchPendingResumeRecordsOrReleasesClaim(t *testing.T) {
	repo := &fakeResumeRepo{pending: &domain.ResumeStart{ID: "outbox-1", JobID: "job-1"}}
	svc := &Service{resumeRepo: repo, resumeLauncher: fakeResumeLauncher{}}
	if err := svc.DispatchPendingResume(context.Background()); err != nil || repo.started != "conductor-1" || repo.released != "" {
		t.Fatalf("successful resume was not recorded: err=%v repo=%+v", err, repo)
	}
	repo.pending = &domain.ResumeStart{ID: "outbox-2", JobID: "job-2"}
	svc.resumeLauncher = fakeResumeLauncher{err: errors.New("conductor unavailable")}
	if err := svc.DispatchPendingResume(context.Background()); err == nil || !strings.Contains(repo.released, "conductor unavailable") {
		t.Fatalf("failed resume claim was not released: err=%v repo=%+v", err, repo)
	}
}

func TestMergeResumeInputsOnlyRequiresPendingSlots(t *testing.T) {
	original := domain.JobInput{Product: &domain.ProductData{ID: "prd"}, VirtualIP: &domain.VirtualIpData{ID: "vip"}}
	needed := map[string]wfdomain.SlotSnapshot{
		"movement": {Name: "movement", ResourceType: "MATERIAL", AssetKind: "movement"},
	}
	if _, err := mergeResumeInputs(original, domain.JobInput{}, needed); err == nil || !strings.Contains(err.Error(), "movement") {
		t.Fatalf("missing pending input was not identified: %v", err)
	}
	patch := domain.JobInput{Materials: []domain.MaterialData{{
		Name: "movement", AssetKind: "movement", URL: "http://media-storage.example/media/materials/org/movement.mp4", Content: "walk",
	}}}
	merged, err := mergeResumeInputs(original, patch, needed)
	if err != nil || len(merged.Materials) != 1 || merged.Product.ID != "prd" {
		t.Fatalf("pending input not merged: input=%+v err=%v", merged, err)
	}
	patch.Materials[0].AssetKind = "pose"
	if _, err := mergeResumeInputs(original, patch, needed); err == nil {
		t.Fatal("wrong frozen asset kind accepted")
	}
	patch.Materials[0].AssetKind = "movement"
	patch.Materials[0].URL = "file:///etc/passwd"
	if _, err := mergeResumeInputs(original, patch, needed); err == nil {
		t.Fatal("local file URL accepted")
	}
	patch.Materials[0].URL = "http://media-storage.example/media/materials/org/movement.mp4"
	patch.Materials[0].Name = "pose"
	if _, err := mergeResumeInputs(original, patch, needed); err == nil {
		t.Fatal("input used only by restored steps accepted")
	}
}

func TestResumeObjectPathRejectsUntrustedAndTraversalURLs(t *testing.T) {
	for _, raw := range []string{
		"http://evil.example/media/materials/org/movement.mp4",
		"http://media-storage.example/media/materials/other/movement.mp4",
		"http://media-storage.example/media/materials/org/../other/movement.mp4",
		"file:///media/materials/org/movement.mp4",
	} {
		if _, err := resumeObjectPath(raw, "http://media-storage.example", "org"); err == nil {
			t.Fatalf("untrusted object URL was accepted: %q", raw)
		}
	}
}

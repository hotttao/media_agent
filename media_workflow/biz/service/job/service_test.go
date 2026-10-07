package job

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/workflow/executable"
)

func TestConductorInputsUsesProductImageURL(t *testing.T) {
	input := domain.JobInput{Product: &domain.ProductData{
		ID: "product-1", ImageURL: "https://storage.example/products/main.png",
	}}
	product, ok := conductorInputs(input)["product"].(map[string]any)
	if !ok {
		t.Fatal("product input is missing")
	}
	if got := product["asset_ref"]; got != input.Product.ImageURL {
		t.Fatalf("asset_ref = %v, want product image URL", got)
	}
}

// —— mock repos ——

type mockJobRepo struct {
	jobs     map[string]*domain.Job
	stepRuns map[string][]*domain.StepRun
}

func (m *mockJobRepo) Save(_ context.Context, j *domain.Job, srs []*domain.StepRun) error {
	j.SubmittedAt = time.Now()
	m.jobs[j.ID] = j
	m.stepRuns[j.ID] = srs
	return nil
}
func (m *mockJobRepo) SaveImported(ctx context.Context, j *domain.Job, srs []*domain.StepRun) error {
	return m.Save(ctx, j, srs)
}
func (m *mockJobRepo) GetByLegacySource(_ context.Context, workspaceID, sourceSystem, sourceRecordID string) (*domain.Job, error) {
	for _, j := range m.jobs {
		if j.WorkspaceID == workspaceID && j.SourceSystem == sourceSystem && j.SourceRecordID == sourceRecordID {
			return j, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockJobRepo) Get(_ context.Context, id string) (*domain.Job, error) {
	j, ok := m.jobs[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return j, nil
}
func (m *mockJobRepo) GetWithStepRuns(_ context.Context, id string) (*domain.Job, []*domain.StepRun, error) {
	j, ok := m.jobs[id]
	if !ok {
		return nil, nil, gorm.ErrRecordNotFound
	}
	return j, m.stepRuns[id], nil
}
func (m *mockJobRepo) GetByConductorWorkflowID(_ context.Context, workflowID string) (*domain.Job, error) {
	for _, item := range m.jobs {
		if item.ConductorWorkflowID == workflowID {
			return item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockJobRepo) List(_ context.Context, q domain.ListQuery) ([]*domain.Job, int, error) {
	var out []*domain.Job
	for _, j := range m.jobs {
		out = append(out, j)
	}
	return out, len(out), nil
}
func (m *mockJobRepo) UpdateStatus(_ context.Context, id string, status domain.JobStatus, failureReason string) error {
	j, ok := m.jobs[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	j.Status = status
	j.FailureReason = failureReason
	if status.IsTerminal() {
		now := time.Now()
		j.FinishedAt = &now
	}
	return nil
}
func (m *mockJobRepo) UpdateInput(_ context.Context, id string, input domain.JobInput) error {
	m.jobs[id].Input = input
	return nil
}
func (m *mockJobRepo) AppendRetryHistory(_ context.Context, id string, record domain.RetryRecord, retryCount int) error {
	j := m.jobs[id]
	j.RetryHistory = append(j.RetryHistory, record)
	j.RetryCount = retryCount
	return nil
}

func (m *mockJobRepo) UpdateConductorExecution(_ context.Context, id, workflowID, hash string) error {
	job := m.jobs[id]
	job.ConductorWorkflowID = workflowID
	job.CompilationHash = hash
	return nil
}

type mockStepRunRepo struct {
	items map[string]*domain.StepRun
}

func (m *mockStepRunRepo) Get(_ context.Context, id string) (*domain.StepRun, error) {
	sr, ok := m.items[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return sr, nil
}
func (m *mockStepRunRepo) ListByJob(_ context.Context, jobID string) ([]*domain.StepRun, error) {
	var out []*domain.StepRun
	for _, sr := range m.items {
		if sr.JobID == jobID {
			out = append(out, sr)
		}
	}
	return out, nil
}
func (m *mockStepRunRepo) GetByJobAndStepKey(_ context.Context, jobID, stepKey string) (*domain.StepRun, error) {
	for _, item := range m.items {
		if item.JobID == jobID && item.StepKey == stepKey {
			return item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockStepRunRepo) ListByJobAndStatus(_ context.Context, jobID string, status domain.StepRunStatus) ([]*domain.StepRun, error) {
	var out []*domain.StepRun
	for _, sr := range m.items {
		if sr.JobID == jobID && sr.Status == status {
			out = append(out, sr)
		}
	}
	return out, nil
}
func (m *mockStepRunRepo) ClaimAtomic(_ context.Context, q domain.ClaimQuery) (*domain.StepRun, error) {
	for _, sr := range m.items {
		if sr.Status == domain.SRPending && sr.Platform == q.Platform {
			sr.Status = domain.SRClaimed
			sr.ClaimedBy = q.WorkerID
			now := time.Now()
			sr.ClaimedAt = &now
			return sr, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockStepRunRepo) UpdateStatus(_ context.Context, id string, status domain.StepRunStatus, updates map[string]any) error {
	sr := m.items[id]
	sr.Status = status
	if status == domain.SRSucceeded || status == domain.SRFailed {
		now := time.Now()
		sr.FinishedAt = &now
	}
	if updates != nil {
		if count, ok := updates["retry_count"].(int); ok {
			sr.RetryCount = count
		}
		if v, ok := updates["selected_index"]; ok {
			if idx, ok2 := v.(int); ok2 {
				sr.SelectedIndex = &idx
			}
		}
	}
	return nil
}
func (m *mockStepRunRepo) UpdateResolvedInputs(_ context.Context, id string, inputs map[string]any, prompt string) error {
	m.items[id].ResolvedInputs = inputs
	m.items[id].RenderedPrompt = prompt
	return nil
}
func (m *mockStepRunRepo) UpdateCandidates(_ context.Context, id string, candidates []domain.Candidate) error {
	m.items[id].Candidates = candidates
	return nil
}
func (m *mockStepRunRepo) UpdateOutput(_ context.Context, id string, outputAssetID string) error {
	m.items[id].OutputAssetID = outputAssetID
	return nil
}
func (m *mockStepRunRepo) UpdateFailure(_ context.Context, id string, reason string) error {
	m.items[id].FailureReason = reason
	return nil
}
func (m *mockStepRunRepo) ReclaimStale(_ context.Context, timeoutSeconds int) (int64, error) {
	return 0, nil
}

// mockWorkflowLookup 返回固定 workflow。
type mockWorkflowLookup struct {
	wf *wfdomain.Workflow
}

func (m *mockWorkflowLookup) Get(_ context.Context, id string) (*wfdomain.Workflow, error) {
	if m.wf == nil || m.wf.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return m.wf, nil
}

// mockTemplateLookup 返回固定 template snapshot。
type mockTemplateLookup struct{}

func (m *mockTemplateLookup) GetVersion(_ context.Context, id string, version int) (*wfdomain.TemplateSnapshot, error) {
	return &wfdomain.TemplateSnapshot{
		ID: id, Scope: "ai_clothing", Status: "draft",
		PromptBody: "test prompt {{.scene.url}}", OutputAssetKind: "first_frame",
		Platform: "gpt", GenWay: "i2i",
		NeedsConfirm: false, CandidateCount: 1,
		Inputs: []wfdomain.SlotSnapshot{{Name: "scene", ResourceType: "MATERIAL", Required: true}},
	}, nil
}

// mockMaterialsClient 不实际调 HTTP。
type mockMaterialsClient struct{}

func (m *mockMaterialsClient) Dummy() {}
func (m *mockMaterialsClient) UpsertGeneratedAsset(_ context.Context, req *client.UpsertGenAssetReq) (string, error) {
	return "gen_fake_id", nil
}

type mockWorkflowEngine struct {
	execution       *domain.EngineExecution
	completedRef    string
	completedOutput map[string]any
	rerunTaskID     string
	rerunInput      map[string]any
	terminated      bool
	correlationID   string
	taskToDomain    map[string]string
	retryCalls      int
	retryErr        error
}

func (m *mockWorkflowEngine) RetryWorkflow(context.Context, string) error {
	m.retryCalls++
	if m.retryErr == nil {
		m.execution.Status = "RUNNING"
	}
	return m.retryErr
}

func (m *mockWorkflowEngine) Start(_ context.Context, _ string, _ int, correlationID string, _ map[string]any, taskToDomain map[string]string) (string, error) {
	m.correlationID = correlationID
	m.taskToDomain = taskToDomain
	return "execution-1", nil
}
func (m *mockWorkflowEngine) Get(context.Context, string) (*domain.EngineExecution, error) {
	return m.execution, nil
}
func (m *mockWorkflowEngine) CompleteTask(_ context.Context, _ string, reference string, output map[string]any) error {
	m.completedRef = reference
	m.completedOutput = output
	return nil
}
func (m *mockWorkflowEngine) RerunFromTask(_ context.Context, _ string, taskID string) error {
	m.rerunTaskID = taskID
	return nil
}
func (m *mockWorkflowEngine) RerunWorkflow(_ context.Context, _ string, input map[string]any) error {
	m.rerunInput = input
	return nil
}
func (m *mockWorkflowEngine) Terminate(context.Context, string, string) error {
	m.terminated = true
	return nil
}

// sharedAssetKindInfo alias（避免 import shared）

// —— helpers ——

func newTestService(needsConfirm bool) (*Service, *mockJobRepo, *mockStepRunRepo) {
	jobRepo := &mockJobRepo{jobs: map[string]*domain.Job{}, stepRuns: map[string][]*domain.StepRun{}}
	srRepo := &mockStepRunRepo{items: map[string]*domain.StepRun{}}
	wfLookup := &mockWorkflowLookup{wf: &wfdomain.Workflow{
		ID: "wf1", Scope: "ai_clothing", Version: 1, OutputStepKey: "s1",
		Steps: []wfdomain.Step{{Key: "s1", Order: 1, TemplateID: "tpl1", TemplateVersion: 1,
			SlotBindings: []wfdomain.SlotBinding{{ToInput: "scene", Source: wfdomain.SourceWorkflowInput, Ref: "scene"}}}},
		InputSchema: []wfdomain.InputSchemaEntry{{Name: "scene", ResourceType: "MATERIAL", Required: true}},
	}}
	tplLookup := &mockTplConfirm{needsConfirm: needsConfirm}
	svc := NewService(jobRepo, srRepo, nil, wfLookup, tplLookup, &mockMaterialsClient{}, nil, nil, nil)
	svc.resumeStorageOrigin = "https://example.test"
	svc.resumeURLVerifier = func(context.Context, string) error { return nil }
	return svc, jobRepo, srRepo
}

// mockTplConfirm 可配置 needs_confirm。
type mockTplConfirm struct{ needsConfirm bool }

func (m *mockTplConfirm) GetVersion(_ context.Context, id string, version int) (*wfdomain.TemplateSnapshot, error) {
	return &wfdomain.TemplateSnapshot{
		ID: id, Scope: "ai_clothing", Status: "draft",
		PromptBody: "test {{.scene.url}}", OutputAssetKind: "first_frame",
		Platform: "gpt", GenWay: "i2i",
		NeedsConfirm: m.needsConfirm, CandidateCount: 4,
		Inputs: []wfdomain.SlotSnapshot{{Name: "scene", ResourceType: "MATERIAL", Required: true}},
	}, nil
}

func makeClaimedStepRun(repo *mockStepRunRepo, needsConfirm bool) *domain.StepRun {
	sr := &domain.StepRun{
		ID: "sr1", JobID: "job1", StepKey: "s1", StepOrder: 1,
		TemplateID: "tpl1", TemplateVersion: 1, Status: domain.SRClaimed,
		Platform: "gpt", GenWay: "i2i", AssetKind: "first_frame",
		NeedsConfirm: needsConfirm, CandidateCount: 4,
		ResolvedInputs: map[string]any{}, Candidates: []domain.Candidate{},
	}
	repo.items["sr1"] = sr
	return sr
}

// —— tests ——

func TestSubmit_OmittedWorkflowVersionUsesCurrent(t *testing.T) {
	svc, _, _ := newTestService(false)
	svc.wfRepo.(*mockWorkflowLookup).wf.CreatedBy = "u1"
	authority := &submitAuthority{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "org"}, allowed: true}
	svc.historyWorkspaces = authority
	svc.resumeAccess = authority

	job, err := svc.Submit(context.Background(), SubmitCommand{
		OrganizationID: "org", WorkspaceID: "workspace",
		WorkflowID: "wf1",
		Input: domain.JobInput{Materials: []domain.MaterialData{{
			Name: "scene", URL: "https://example.test/media/materials/org/scene.png",
		}}},
		SubmittedBy: "u1",
	})
	if err != nil {
		t.Fatalf("submit without workflow_version: %v", err)
	}
	if job.WorkflowVersion != 1 {
		t.Fatalf("workflow version = %d, want 1", job.WorkflowVersion)
	}
}

func TestSubmit_ExplicitStaleWorkflowVersionRejected(t *testing.T) {
	svc, _, _ := newTestService(false)
	svc.wfRepo.(*mockWorkflowLookup).wf.CreatedBy = "u1"
	authority := &submitAuthority{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "org"}, allowed: true}
	svc.historyWorkspaces = authority
	svc.resumeAccess = authority

	_, err := svc.Submit(context.Background(), SubmitCommand{
		WorkflowID: "wf1", WorkflowVersion: 2, SubmittedBy: "u1", OrganizationID: "org", WorkspaceID: "workspace",
	})
	if err == nil {
		t.Fatal("expected stale workflow version to be rejected")
	}
}

func TestTaskDomainsForIRSeparatesPersonalAndManagedTargets(t *testing.T) {
	ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, Steps: []executable.ExecutableStepIR{
		{Capability: executable.CapabilityIR{Key: "personal", Version: 1, ConductorTaskDef: "personal_v1", ExecutionTarget: "personal_worker"}},
		{Capability: executable.CapabilityIR{Key: "managed", Version: 1, ConductorTaskDef: "managed_v1", ExecutionTarget: "managed_worker", ManagedPoolKey: "gpu-cn"}},
	}}

	domains, err := taskDomainsForIR(ir, "user-1")
	if err != nil {
		t.Fatalf("taskDomainsForIR() error = %v", err)
	}
	if domains["personal_v1"] != domain.ExecutionDomainForUser("user-1") {
		t.Fatalf("personal domain = %q", domains["personal_v1"])
	}
	if domains["managed_v1"] != domain.ExecutionDomainForManagedPool("gpu-cn") {
		t.Fatalf("managed domain = %q", domains["managed_v1"])
	}
	if domains["personal_v1"] == domains["managed_v1"] {
		t.Fatal("personal and managed tasks must not share a domain")
	}
}

func TestTaskDomainsForIRRejectsInvalidOrConflictingTargets(t *testing.T) {
	tests := []struct {
		name string
		ir   executable.ExecutableWorkflowIR
		want string
	}{
		{
			name: "managed pool required",
			ir: executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, Steps: []executable.ExecutableStepIR{{
				Capability: executable.CapabilityIR{Key: "managed", Version: 1, ConductorTaskDef: "managed_v1", ExecutionTarget: "managed_worker"},
			}}},
			want: "requires managed pool",
		},
		{
			name: "unknown target",
			ir: executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, Steps: []executable.ExecutableStepIR{{
				Capability: executable.CapabilityIR{Key: "bad", Version: 1, ConductorTaskDef: "bad_v1", ExecutionTarget: "remote"},
			}}},
			want: "invalid execution target",
		},
		{
			name: "same task definition in different domains",
			ir: executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, Steps: []executable.ExecutableStepIR{
				{Capability: executable.CapabilityIR{Key: "personal", Version: 1, ConductorTaskDef: "shared_v1", ExecutionTarget: "personal_worker"}},
				{Capability: executable.CapabilityIR{Key: "managed", Version: 1, ConductorTaskDef: "shared_v1", ExecutionTarget: "managed_worker", ManagedPoolKey: "gpu-cn"}},
			}},
			want: "conflicting execution domains",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := taskDomainsForIR(tt.ir, "user-1")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("taskDomainsForIR() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTaskDomainsForIRTreatsLegacyIRAsPersonal(t *testing.T) {
	ir := executable.ExecutableWorkflowIR{SchemaVersion: 2, Steps: []executable.ExecutableStepIR{{
		Capability: executable.CapabilityIR{Key: "legacy", Version: 1, ConductorTaskDef: "legacy_v1"},
	}}}
	domains, err := taskDomainsForIR(ir, "user-1")
	if err != nil {
		t.Fatalf("taskDomainsForIR() error = %v", err)
	}
	if domains["legacy_v1"] != domain.ExecutionDomainForUser("user-1") {
		t.Fatalf("legacy domain = %q", domains["legacy_v1"])
	}
}

func TestComplete_NoConfirm_DirectSucceeded(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobRunning, SubmittedBy: "u1", Input: domain.JobInput{Materials: []domain.MaterialData{{Name: "scene", URL: "https://x.png"}}}}
	_ = makeClaimedStepRun(srRepo, false)

	cands := []domain.Candidate{{Index: 1, URL: "https://out.png", IsPrimary: true}}
	_, err := svc.Complete(context.Background(), "sr1", cands)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, _ := srRepo.Get(context.Background(), "sr1")
	if updated.Status != domain.SRSucceeded {
		t.Errorf("status = %s, want succeeded", updated.Status)
	}
	if updated.OutputAssetID == "" {
		t.Error("expected non-empty output_asset_id")
	}
}

func TestComplete_NeedsConfirm_PausesForUser(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(true)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobRunning, SubmittedBy: "u1", Input: domain.JobInput{}}
	_ = makeClaimedStepRun(srRepo, true)

	cands := []domain.Candidate{{Index: 1, URL: "https://a.png"}, {Index: 2, URL: "https://b.png"}}
	_, err := svc.Complete(context.Background(), "sr1", cands)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, _ := srRepo.Get(context.Background(), "sr1")
	if updated.Status != domain.SRNeedConfirm {
		t.Errorf("status = %s, want need_confirm", updated.Status)
	}
	if len(updated.Candidates) != 2 {
		t.Errorf("candidates = %d, want 2", len(updated.Candidates))
	}
	// Job should also be need_confirm
	job, _ := jobRepo.Get(context.Background(), "job1")
	if job.Status != domain.JobNeedConfirm {
		t.Errorf("job status = %s, want need_confirm", job.Status)
	}
}

func TestConfirm_SelectCandidate_Succeeds(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(true)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobNeedConfirm, SubmittedBy: "u1", Input: domain.JobInput{}}
	sr := makeClaimedStepRun(srRepo, true)
	sr.Status = domain.SRNeedConfirm
	sr.Candidates = []domain.Candidate{{Index: 1, URL: "https://a.png"}, {Index: 2, URL: "https://b.png"}}

	_, err := svc.Confirm(context.Background(), "sr1", 1) // 选第二个
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, _ := srRepo.Get(context.Background(), "sr1")
	if updated.Status != domain.SRSucceeded {
		t.Errorf("status = %s, want succeeded", updated.Status)
	}
	if updated.OutputAssetID == "" {
		t.Error("expected output_asset_id after confirm")
	}
}

func TestConfirm_OutOfRange(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(true)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobNeedConfirm, SubmittedBy: "u1", Input: domain.JobInput{}}
	sr := makeClaimedStepRun(srRepo, true)
	sr.Status = domain.SRNeedConfirm
	sr.Candidates = []domain.Candidate{{Index: 1, URL: "https://a.png"}}

	_, err := svc.Confirm(context.Background(), "sr1", 5)
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestRegenerate_BackToPending(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(true)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobNeedConfirm, SubmittedBy: "u1", Input: domain.JobInput{}}
	sr := makeClaimedStepRun(srRepo, true)
	sr.Status = domain.SRNeedConfirm
	sr.Candidates = []domain.Candidate{{Index: 1, URL: "https://a.png"}}

	_, err := svc.Regenerate(context.Background(), "sr1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, _ := srRepo.Get(context.Background(), "sr1")
	if updated.Status != domain.SRPending {
		t.Errorf("status = %s, want pending", updated.Status)
	}
}

func TestFail_StepRunFails_JobFails(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobRunning, SubmittedBy: "u1", Input: domain.JobInput{}}
	makeClaimedStepRun(srRepo, false)

	_, err := svc.Fail(context.Background(), "sr1", "generation timeout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, _ := srRepo.Get(context.Background(), "sr1")
	if updated.Status != domain.SRFailed {
		t.Errorf("step status = %s, want failed", updated.Status)
	}
	job, _ := jobRepo.Get(context.Background(), "job1")
	if job.Status != domain.JobFailed {
		t.Errorf("job status = %s, want failed", job.Status)
	}
	if job.FailureReason != "generation timeout" {
		t.Errorf("failure_reason = %s, want 'generation timeout'", job.FailureReason)
	}
}

func TestCancelJob(t *testing.T) {
	svc, jobRepo, _ := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", Status: domain.JobRunning}

	j, err := svc.Cancel(context.Background(), "job1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if j.Status != domain.JobCancelled {
		t.Errorf("status = %s, want cancelled", j.Status)
	}
}

func TestRetry_NotFailedRejected(t *testing.T) {
	svc, jobRepo, _ := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobRunning}

	_, err := svc.Retry(context.Background(), "job1", "retry", nil)
	if err == nil {
		t.Fatal("expected error for retrying non-failed job")
	}
}

func TestRetryRejectsExecutionWithoutConductor(t *testing.T) {
	svc, jobs, steps := newTestService(false)
	jobs.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobFailed}
	sr := makeClaimedStepRun(steps, false)
	sr.Status = domain.SRFailed
	if _, err := svc.Retry(context.Background(), "job1", "retry", nil); err == nil {
		t.Fatal("retry fell back to a local scheduler")
	}
	if sr.Status != domain.SRFailed || jobs.jobs["job1"].Status != domain.JobFailed {
		t.Fatal("unbound execution was mutated")
	}
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
	svc, jobRepo, _ := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobFailed, RetryCount: 3}

	_, err := svc.Retry(context.Background(), "job1", "retry", nil)
	if err == nil {
		t.Fatal("expected error for max retries exceeded")
	}
}

func TestValidateBundle_MissingRequired(t *testing.T) {
	svc := &Service{}
	schema := []wfdomain.InputSchemaEntry{
		{Name: "scene", ResourceType: "MATERIAL", Required: true},
		{Name: "pose", ResourceType: "MATERIAL", Required: true},
	}
	input := domain.JobInput{
		Materials: []domain.MaterialData{{Name: "scene", URL: "https://x.png"}},
		// 缺 pose
	}
	err := svc.validateBundle(schema, input)
	if err == nil {
		t.Fatal("expected error for missing required material")
	}
}

func TestValidateBundle_AllPresent(t *testing.T) {
	svc := &Service{}
	schema := []wfdomain.InputSchemaEntry{
		{Name: "scene", ResourceType: "MATERIAL", Required: true},
		{Name: "pose", ResourceType: "MATERIAL", Required: false}, // 可选
	}
	input := domain.JobInput{
		Materials: []domain.MaterialData{{Name: "scene", URL: "https://x.png"}},
	}
	err := svc.validateBundle(schema, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBundle_RequiredProduct(t *testing.T) {
	svc := &Service{}
	schema := []wfdomain.InputSchemaEntry{
		{Name: "product", ResourceType: "PRODUCT", Required: true},
	}
	input := domain.JobInput{} // 缺 product
	err := svc.validateBundle(schema, input)
	if err == nil {
		t.Fatal("expected error for missing required product")
	}
}

// 闸2 补全：material name 唯一校验
func TestValidateBundle_DuplicateMaterialName(t *testing.T) {
	svc := &Service{}
	schema := []wfdomain.InputSchemaEntry{
		{Name: "scene", ResourceType: "MATERIAL", Required: true},
	}
	input := domain.JobInput{
		Materials: []domain.MaterialData{
			{Name: "scene", URL: "https://a.png"},
			{Name: "scene", URL: "https://b.png"}, // 重复
		},
	}
	err := svc.validateBundle(schema, input)
	if err == nil {
		t.Fatal("expected error for duplicate material name")
	}
}

// 闸2 补全：asset name 唯一校验
func TestValidateBundle_DuplicateAssetName(t *testing.T) {
	svc := &Service{}
	schema := []wfdomain.InputSchemaEntry{
		{Name: "model_image", ResourceType: "ASSET", Required: true},
	}
	input := domain.JobInput{
		Assets: []domain.AssetData{
			{Name: "model_image", URL: "https://a.png"},
			{Name: "model_image", URL: "https://b.png"}, // 重复
		},
	}
	err := svc.validateBundle(schema, input)
	if err == nil {
		t.Fatal("expected error for duplicate asset name")
	}
}

// 闸2 补全：无多余类型校验（bundle 含 schema 未声明的 material）
func TestValidateBundle_ExtraMaterialNotInSchema(t *testing.T) {
	svc := &Service{}
	schema := []wfdomain.InputSchemaEntry{
		{Name: "scene", ResourceType: "MATERIAL", Required: true},
	}
	input := domain.JobInput{
		Materials: []domain.MaterialData{
			{Name: "scene", URL: "https://a.png"},
			{Name: "pose", URL: "https://b.png"}, // schema 未声明
		},
	}
	err := svc.validateBundle(schema, input)
	if err == nil {
		t.Fatal("expected error for extra material not in schema")
	}
}

// 闸2 补全：无多余类型校验（bundle material 与 schema resource_type 不匹配）
func TestValidateBundle_ResourceTypeMismatch(t *testing.T) {
	svc := &Service{}
	schema := []wfdomain.InputSchemaEntry{
		{Name: "model_image", ResourceType: "ASSET", Required: true},
	}
	// bundle 把 model_image 当 MATERIAL，但 schema 声明为 ASSET
	input := domain.JobInput{
		Materials: []domain.MaterialData{
			{Name: "model_image", URL: "https://a.png"},
		},
	}
	err := svc.validateBundle(schema, input)
	if err == nil {
		t.Fatal("expected error for resource_type mismatch")
	}
}

func TestValidateBundle_AssetKindMismatch(t *testing.T) {
	svc := &Service{}
	schema := []wfdomain.InputSchemaEntry{
		{Name: "scene", ResourceType: "MATERIAL", AssetKind: "scene", Required: true},
	}
	input := domain.JobInput{Materials: []domain.MaterialData{
		{Name: "scene", AssetKind: "pose", URL: "https://a.png"},
	}}
	if err := svc.validateBundle(schema, input); err == nil {
		t.Fatal("expected error for asset_kind mismatch")
	}
}

// FailStepRun cascade skip 下游：A failed -> B (BLOCKED 依赖 A) -> SKIPPED
func TestFail_CascadeSkipDownstream(t *testing.T) {
	svc, jobRepo, srRepo := newTestServiceWithDAG()
	// stepA: claimed（worker 领取）
	srA := &domain.StepRun{
		ID: "sr_a", JobID: "job1", StepKey: "step_a", StepOrder: 1,
		TemplateID: "tpl1", TemplateVersion: 1, Status: domain.SRClaimed,
		Platform: "gpt", GenWay: "i2i", AssetKind: "first_frame",
		NeedsConfirm: false, CandidateCount: 1,
	}
	// stepB: blocked，依赖 step_a 输出
	srB := &domain.StepRun{
		ID: "sr_b", JobID: "job1", StepKey: "step_b", StepOrder: 2,
		TemplateID: "tpl1", TemplateVersion: 1, Status: domain.SRBlocked,
		Platform: "gpt", GenWay: "i2i", AssetKind: "first_frame",
		NeedsConfirm: false, CandidateCount: 1,
	}
	srRepo.items["sr_a"] = srA
	srRepo.items["sr_b"] = srB
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobRunning, SubmittedBy: "u1", Input: domain.JobInput{}}

	_, err := svc.Fail(context.Background(), "sr_a", "generation timeout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// step_a 应 failed
	a, _ := srRepo.Get(context.Background(), "sr_a")
	if a.Status != domain.SRFailed {
		t.Errorf("step_a status = %s, want failed", a.Status)
	}
	// step_b 应 cascade skipped
	b, _ := srRepo.Get(context.Background(), "sr_b")
	if b.Status != domain.SRSkipped {
		t.Errorf("step_b status = %s, want skipped (cascade)", b.Status)
	}
}

// FailStepRun 状态守卫：succeeded step 不可 fail
func TestFail_AlreadySucceeded(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobRunning, SubmittedBy: "u1", Input: domain.JobInput{}}
	sr := makeClaimedStepRun(srRepo, false)
	sr.Status = domain.SRSucceeded

	_, err := svc.Fail(context.Background(), "sr1", "too late")
	if err == nil {
		t.Fatal("expected error for failing already-succeeded step")
	}
}

// Cancel 状态守卫：succeeded job 不可 cancel
func TestCancel_TerminalJob(t *testing.T) {
	svc, jobRepo, _ := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobSucceeded}

	_, err := svc.Cancel(context.Background(), "job1")
	if err == nil {
		t.Fatal("expected error for cancelling terminal job")
	}
}

// advanceDAG 多级 cascade：A failed -> B (依赖 A) skipped -> C (依赖 B) skipped
func TestAdvanceDAG_MultiLevelCascade(t *testing.T) {
	svc, jobRepo, srRepo := newTestServiceWithDAG()
	srA := &domain.StepRun{ID: "sr_a", JobID: "job1", StepKey: "step_a", StepOrder: 1, Status: domain.SRFailed, Platform: "gpt", GenWay: "i2i"}
	srB := &domain.StepRun{ID: "sr_b", JobID: "job1", StepKey: "step_b", StepOrder: 2, Status: domain.SRBlocked, Platform: "gpt", GenWay: "i2i"}
	srC := &domain.StepRun{ID: "sr_c", JobID: "job1", StepKey: "step_c", StepOrder: 3, Status: domain.SRBlocked, Platform: "gpt", GenWay: "i2i"}
	srRepo.items["sr_a"] = srA
	srRepo.items["sr_b"] = srB
	srRepo.items["sr_c"] = srC
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobFailed, SubmittedBy: "u1", Input: domain.JobInput{}}

	// 触发 advanceDAG（通过 Fail 间接调用，但 step_a 已 failed，直接调内部方法测）
	svc.advanceDAG(context.Background(), "job1", "step_a")

	b, _ := srRepo.Get(context.Background(), "sr_b")
	if b.Status != domain.SRSkipped {
		t.Errorf("step_b status = %s, want skipped", b.Status)
	}
	c, _ := srRepo.Get(context.Background(), "sr_c")
	if c.Status != domain.SRSkipped {
		t.Errorf("step_c status = %s, want skipped (multi-level cascade)", c.Status)
	}
}

// Complete 多候选分支：必须先等待用户，再把选中的 candidate 落库。
func TestComplete_MultipleCandidatesRequiresApproval(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobRunning, SubmittedBy: "u1", Input: domain.JobInput{Materials: []domain.MaterialData{{Name: "scene", URL: "https://x.png"}}}}
	_ = makeClaimedStepRun(srRepo, false)

	// 2 候选，第 2 个是 primary
	cands := []domain.Candidate{
		{Index: 1, URL: "https://out1.png"},
		{Index: 2, URL: "https://out2.png", IsPrimary: true},
	}
	_, err := svc.Complete(context.Background(), "sr1", cands)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, _ := srRepo.Get(context.Background(), "sr1")
	if updated.Status != domain.SRNeedConfirm {
		t.Errorf("status = %s, want need_confirm", updated.Status)
	}
	if updated.SelectedIndex != nil {
		t.Errorf("selected_index = %v, want nil before approval", updated.SelectedIndex)
	}
	if _, err := svc.Confirm(context.Background(), "sr1", 1); err != nil {
		t.Fatalf("confirm candidate: %v", err)
	}
	updated, _ = srRepo.Get(context.Background(), "sr1")
	if updated.Status != domain.SRSucceeded {
		t.Errorf("status after confirm = %s, want succeeded", updated.Status)
	}
	if updated.SelectedIndex == nil || *updated.SelectedIndex != 1 {
		t.Errorf("selected_index = %v, want 1 after approval", updated.SelectedIndex)
	}
}

func TestConductorApprovalConfirmAndFinalize(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(false)
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{
		Status: "RUNNING",
		Tasks: []domain.EngineTask{
			{TaskID: "invoke-1", ReferenceTaskName: "step__s1__invoke", Status: "COMPLETED", Output: map[string]any{
				"candidates": []any{
					map[string]any{"index": float64(0), "url": "a.png"},
					map[string]any{"index": float64(1), "url": "b.png"},
				},
			}},
			{TaskID: "approval-1", ReferenceTaskName: "step__s1__approval", Status: "IN_PROGRESS", Input: map[string]any{
				"candidates": []any{
					map[string]any{"index": float64(0), "url": "a.png"},
					map[string]any{"index": float64(1), "url": "b.png"},
				},
			}},
		},
	}}
	svc.engine = engine
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Scope: "ai_clothing", WorkflowID: "wf1", Status: domain.JobRunning, SubmittedBy: "u1", ConductorWorkflowID: "execution-1"}
	sr := makeClaimedStepRun(srRepo, false)
	sr.Status = domain.SRBlocked
	jobRepo.stepRuns["job1"] = []*domain.StepRun{sr}

	job, steps, err := svc.Get(context.Background(), "job1")
	if err != nil || job.Status != domain.JobNeedConfirm || steps[0].Status != domain.SRNeedConfirm || len(steps[0].Candidates) != 2 {
		t.Fatalf("approval was not reconciled: job=%#v steps=%#v err=%v", job, steps, err)
	}
	if _, err := svc.Confirm(context.Background(), "sr1", 1); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if engine.completedRef != "step__s1__approval" {
		t.Fatalf("completed ref = %q", engine.completedRef)
	}
	selected := engine.completedOutput["selected_candidate"].(domain.Candidate)
	if selected.URL != "b.png" {
		t.Fatalf("selected candidate = %#v", selected)
	}

	engine.execution = &domain.EngineExecution{Status: "COMPLETED", Tasks: []domain.EngineTask{{
		ReferenceTaskName: "step__s1__finalize", Status: "COMPLETED", Output: map[string]any{
			"output_resource":    map[string]any{"url": "b.png", "content": "approved"},
			"output_resource_id": "gen_fake_id",
		},
	}, {TaskID: "invoke-1", ReferenceTaskName: "step__s1__invoke", Status: "COMPLETED"}}}
	engine.execution.Tasks[0].Output["output_resource"].(map[string]any)["id"] = "gen_fake_id"
	job, steps, err = svc.Get(context.Background(), "job1")
	if err != nil || job.Status != domain.JobSucceeded || steps[0].Status != domain.SRSucceeded || steps[0].OutputAssetID != "gen_fake_id" {
		t.Fatalf("approved step was not finalized: job=%#v steps=%#v err=%v", job, steps, err)
	}
}

func TestConductorRegenerateCompletesCurrentLoopApproval(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(false)
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "RUNNING", Tasks: []domain.EngineTask{{
		TaskID: "approval-2", ReferenceTaskName: "step_7331_approval__2", Status: "IN_PROGRESS",
	}}}}
	svc.engine = engine
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", WorkflowID: "wf1", Status: domain.JobNeedConfirm, ConductorWorkflowID: "execution-1"}
	sr := makeClaimedStepRun(srRepo, false)
	sr.Status = domain.SRNeedConfirm
	sr.Candidates = []domain.Candidate{{Index: 0, URL: "a.png"}}

	updated, err := svc.Regenerate(context.Background(), "sr1")
	if err != nil || engine.rerunTaskID != "" || engine.terminated || engine.completedRef != "step_7331_approval__2" || engine.completedOutput["decision"] != "regenerate" || updated.Status != domain.SRBlocked || len(updated.Candidates) != 0 {
		t.Fatalf("regenerate failed: updated=%#v completed=%q err=%v", updated, engine.completedRef, err)
	}
}

func TestConductorCancelTerminatesExecution(t *testing.T) {
	svc, jobRepo, _ := newTestService(false)
	engine := &mockWorkflowEngine{}
	svc.engine = engine
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobRunning, ConductorWorkflowID: "execution-1"}

	job, err := svc.Cancel(context.Background(), "job1")
	if err != nil || !engine.terminated || job.Status != domain.JobCancelled {
		t.Fatalf("cancel failed: job=%#v terminated=%v err=%v", job, engine.terminated, err)
	}
}

func TestConductorRetryContinuesWithoutResettingSteps(t *testing.T) {
	svc, jobs, steps := newTestService(false)
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "FAILED"}}
	svc.engine = engine
	jobs.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobFailed, ConductorWorkflowID: "execution-1"}
	sr := makeClaimedStepRun(steps, false)
	sr.Status = domain.SRFailed
	sr.Candidates = []domain.Candidate{{Content: "retained"}}
	j, err := svc.Retry(context.Background(), "job1", "retry", nil)
	if err != nil || engine.retryCalls != 1 || engine.rerunInput != nil || engine.rerunTaskID != "" ||
		j.Status != domain.JobRunning || sr.Status != domain.SRFailed || len(sr.Candidates) != 1 ||
		j.RetryCount != 0 || len(j.RetryHistory) != 1 {
		t.Fatalf("native retry reset state or reran render: err=%v", err)
	}
}

// newTestServiceWithDAG 创建一个 2-step DAG（step_a -> step_b -> step_c）用于 cascade 测试。
func newTestServiceWithDAG() (*Service, *mockJobRepo, *mockStepRunRepo) {
	jobRepo := &mockJobRepo{jobs: map[string]*domain.Job{}, stepRuns: map[string][]*domain.StepRun{}}
	srRepo := &mockStepRunRepo{items: map[string]*domain.StepRun{}}
	wfLookup := &mockWorkflowLookup{wf: &wfdomain.Workflow{
		ID: "wf1", Scope: "ai_clothing", OutputStepKey: "step_c",
		Steps: []wfdomain.Step{
			{Key: "step_a", Order: 1, TemplateID: "tpl1", TemplateVersion: 1, SlotBindings: []wfdomain.SlotBinding{}},
			{Key: "step_b", Order: 2, TemplateID: "tpl1", TemplateVersion: 1, SlotBindings: []wfdomain.SlotBinding{{ToInput: "in", Source: wfdomain.SourceStepOutput, Ref: "step_a"}}},
			{Key: "step_c", Order: 3, TemplateID: "tpl1", TemplateVersion: 1, SlotBindings: []wfdomain.SlotBinding{{ToInput: "in", Source: wfdomain.SourceStepOutput, Ref: "step_b"}}},
		},
		InputSchema: []wfdomain.InputSchemaEntry{},
	}}
	tplLookup := &mockTplConfirm{needsConfirm: false}
	svc := NewService(jobRepo, srRepo, nil, wfLookup, tplLookup, &mockMaterialsClient{}, nil, nil, nil)
	return svc, jobRepo, srRepo
}

var _ = errors.New

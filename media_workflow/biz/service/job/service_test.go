package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared/client"
)

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
	j.Status = domain.JobRunning
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
		PromptBody: "test prompt {scene.url}", OutputAssetKind: "first_frame",
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

// sharedAssetKindInfo alias（避免 import shared）

// —— helpers ——

func newTestService(needsConfirm bool) (*Service, *mockJobRepo, *mockStepRunRepo) {
	jobRepo := &mockJobRepo{jobs: map[string]*domain.Job{}, stepRuns: map[string][]*domain.StepRun{}}
	srRepo := &mockStepRunRepo{items: map[string]*domain.StepRun{}}
	wfLookup := &mockWorkflowLookup{wf: &wfdomain.Workflow{
		ID: "wf1", Scope: "ai_clothing", OutputStepKey: "s1",
		Steps: []wfdomain.Step{{Key: "s1", Order: 1, TemplateID: "tpl1", TemplateVersion: 1,
			Bindings: []wfdomain.Binding{{ToInput: "scene", Source: wfdomain.SourceWorkflowInput}}}},
		InputSchema: []wfdomain.InputSchemaEntry{{Name: "scene", ResourceType: "MATERIAL", Required: true}},
	}}
	tplLookup := &mockTplConfirm{needsConfirm: needsConfirm}
	svc := NewService(jobRepo, srRepo, nil, wfLookup, tplLookup, &mockMaterialsClient{}, nil)
	return svc, jobRepo, srRepo
}

// mockTplConfirm 可配置 needs_confirm。
type mockTplConfirm struct{ needsConfirm bool }

func (m *mockTplConfirm) GetVersion(_ context.Context, id string, version int) (*wfdomain.TemplateSnapshot, error) {
	return &wfdomain.TemplateSnapshot{
		ID: id, Scope: "ai_clothing", Status: "draft",
		PromptBody: "test {scene.url}", OutputAssetKind: "first_frame",
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

func TestRetry_FailedResetsSteps(t *testing.T) {
	svc, jobRepo, srRepo := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobFailed, RetryCount: 0, SubmittedBy: "u1"}
	srRepo.items["sr1"] = &domain.StepRun{ID: "sr1", JobID: "job1", StepKey: "s1", Status: domain.SRFailed, Platform: "gpt", GenWay: "i2i"}

	j, err := svc.Retry(context.Background(), "job1", "retry reason", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if j.Status != domain.JobRunning {
		t.Errorf("job status = %s, want running", j.Status)
	}
	if j.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", j.RetryCount)
	}
	sr, _ := srRepo.Get(context.Background(), "sr1")
	if sr.Status != domain.SRPending {
		t.Errorf("step status = %s, want pending", sr.Status)
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

var _ = errors.New

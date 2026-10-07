package job

import (
	"context"
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

type mockWorkflowLookup struct{ wf *wfdomain.Workflow }

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
	if m.execution != nil {
		copy := *m.execution
		copy.Input, copy.Status = input, "RUNNING"
		m.execution = &copy
	}
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
	svc, _, _, _, _, cmd := submissionFixture()
	cmd.WorkflowVersion, cmd.IdempotencyKey = 0, ""
	job, err := svc.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatal("published native submission failed", err)
	}
	if job.WorkflowVersion != 1 || job.Status != domain.JobPending {
		t.Fatal("omitted version did not use published native definition")
	}
}

func TestSubmit_ExplicitStaleWorkflowVersionRejected(t *testing.T) {
	svc, jobs, _, _, _, cmd := submissionFixture()
	cmd.WorkflowVersion = 2
	if _, err := svc.Submit(context.Background(), cmd); err == nil || len(jobs.jobs) != 0 {
		t.Fatal("stale native version accepted", err)
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

func TestCancel_TerminalJob(t *testing.T) {
	svc, jobRepo, _ := newTestService(false)
	jobRepo.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobSucceeded}

	_, err := svc.Cancel(context.Background(), "job1")
	if err == nil {
		t.Fatal("expected error for cancelling terminal job")
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

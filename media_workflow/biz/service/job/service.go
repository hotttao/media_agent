package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/media_workflow/biz/dal"
	capabilitydomain "media_agent/media_workflow/biz/domain/capability"
	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/shared/taskcontext"
	"media_agent/media_workflow/biz/shared/taskref"
	"media_agent/media_workflow/biz/workflow/executable"
)

// GeneratedAssetSaver 落产物端口（由 client.MaterialsClient 实现，main.go 注入）。
type GeneratedAssetSaver interface {
	UpsertGeneratedAsset(ctx context.Context, req *client.UpsertGenAssetReq) (string, error)
}

// Service owns execution commands and read projections; Conductor owns scheduling.
type Service struct {
	workflowAccess      func(context.Context, string, string, string, string) error
	jobRepo             domain.JobRepo
	stepRunRepo         domain.StepRunRepo
	tx                  dal.TxRunner
	wfRepo              domain.WorkflowLookup
	tplLookup           wfdomain.TemplateLookup
	materials           GeneratedAssetSaver
	candidateMedia      CandidateObjectResolver
	historyAssets       ImportedAssetLookup
	historyWorkspaces   HistoricalWorkspaceLookup
	resumeRepo          domain.ResumeRepo
	resumeAccess        ResumeWorkspaceAccess
	resumeLauncher      ResumeLauncher
	resumeStorageOrigin string
	resumeURLVerifier   func(context.Context, string) error
	jobCfg              *configpb.JobConfig
	compilations        wfdomain.CompilationRepo
	engine              domain.WorkflowEngine
	reviews             domain.ReviewRepo
	starts              domain.ExecutionStartRepo
	submissions         domain.SubmissionRepo
}

func NewService(jr domain.JobRepo, sr domain.StepRunRepo, tx dal.TxRunner, wfRepo domain.WorkflowLookup, tl wfdomain.TemplateLookup, mc GeneratedAssetSaver, jobCfg *configpb.JobConfig, compilations wfdomain.CompilationRepo, engine domain.WorkflowEngine) *Service {
	return &Service{jobRepo: jr, stepRunRepo: sr, tx: tx, wfRepo: wfRepo, tplLookup: tl, materials: mc, jobCfg: jobCfg, compilations: compilations, engine: engine}
}

// —— SubmitJob ——
type SubmitCommand struct {
	OrganizationID  string
	WorkspaceID     string
	Delegated       bool
	WorkflowID      string
	WorkflowVersion int
	Input           domain.JobInput
	SubmittedBy     string
	IdempotencyKey  string
}

var ErrSubmitForbidden = errors.New("workflow execution access denied")
var ErrSubmitAuthorityUnavailable = errors.New("workflow execution authorization unavailable")

func (s *Service) Submit(ctx context.Context, cmd SubmitCommand) (*domain.Job, error) {
	{
		if cmd.SubmittedBy == "" || cmd.WorkspaceID == "" || (cmd.Delegated && cmd.OrganizationID == "") {
			return nil, ErrSubmitForbidden
		}
		if s.historyWorkspaces == nil || s.resumeAccess == nil {
			return nil, ErrSubmitAuthorityUnavailable
		}
		workspace, err := s.historyWorkspaces.GetWorkspaceForUser(ctx, cmd.WorkspaceID, cmd.SubmittedBy)
		if err != nil {
			return nil, ErrSubmitAuthorityUnavailable
		}
		if workspace == nil || workspace.ID != cmd.WorkspaceID || workspace.OrganizationID == "" ||
			(cmd.OrganizationID != "" && workspace.OrganizationID != cmd.OrganizationID) {
			return nil, ErrSubmitForbidden
		}
		cmd.OrganizationID = workspace.OrganizationID
		allowed, err := s.resumeAccess.CanUseWorkflow(ctx, cmd.WorkspaceID, cmd.SubmittedBy)
		if err != nil {
			return nil, ErrSubmitAuthorityUnavailable
		}
		if !allowed {
			return nil, ErrSubmitForbidden
		}
	}
	var submission *domain.Submission
	if cmd.IdempotencyKey != "" {
		var previous *domain.Job
		var err error
		submission, previous, err = s.prepareSubmission(ctx, cmd)
		if err != nil {
			return nil, err
		}
		if previous != nil {
			return previous, nil
		}
	}
	// 加载 workflow
	wf, err := s.wfRepo.Get(ctx, cmd.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow %s: %w", cmd.WorkflowID, err)
	}
	if wf.CreatedBy != cmd.SubmittedBy && wf.Visibility != wfdomain.VisibilityPublic {
		if s.workflowAccess == nil {
			return nil, ErrSubmitForbidden
		}
	}
	if s.workflowAccess != nil {
		if err := s.workflowAccess(ctx, wf.ID, cmd.OrganizationID, cmd.WorkspaceID, cmd.SubmittedBy); err != nil {
			return nil, err
		}
	}
	// workflow_version is optional at the HTTP boundary. Zero means "use the
	// current published version"; a non-zero value remains an optimistic lock.
	if cmd.WorkflowVersion != 0 && wf.Version != cmd.WorkflowVersion {
		return nil, fmt.Errorf("workflow version mismatch: want %d, got %d", wf.Version, cmd.WorkflowVersion)
	}
	if s.compilations == nil || s.engine == nil {
		return nil, errors.New("registered Conductor execution unavailable")
	}
	if wf.Status != wfdomain.StatusPublished {
		return nil, fmt.Errorf("workflow %s is not published", wf.ID)
	}
	compilation, err := s.compilations.GetRegistered(ctx, wf.ID, wf.Version)
	if err != nil {
		return nil, fmt.Errorf("registered workflow compilation %s@%d: %w", wf.ID, wf.Version, err)
	}
	// 闸2: bundle vs input_schema
	if err := s.validateBundle(wf.InputSchema, cmd.Input); err != nil {
		return nil, err
	}
	cmd.Input, err = s.freezeInputStorage(ctx, cmd.OrganizationID, cmd.Input)
	if err != nil {
		return nil, err
	}
	// 创建 job + step_runs
	jobID := "job_" + uuid.NewString()
	job := &domain.Job{
		OrganizationID: cmd.OrganizationID, WorkspaceID: cmd.WorkspaceID,
		ID: jobID, Scope: wf.Scope, WorkflowID: wf.ID, WorkflowVersion: wf.Version,
		Input: cmd.Input, Status: domain.JobPending, SubmittedBy: cmd.SubmittedBy,
		SubmittedAt: time.Now(),
	}
	// 为每个 step 创建 step_run，反范式化 template 字段
	stepRuns := make([]*domain.StepRun, 0, len(wf.Steps))
	var frozenIR executable.ExecutableWorkflowIR
	if s.workflowAccess != nil && json.Unmarshal(compilation.IRJSON, &frozenIR) != nil {
		return nil, errors.New("invalid frozen compilation")
	}
	for _, step := range wf.Steps {
		// 加载 template 快照（反范式化 platform/gen_way/asset_kind 等）
		var tpl *wfdomain.TemplateSnapshot
		var err error
		if s.workflowAccess == nil {
			tpl, err = s.tplLookup.GetVersion(ctx, step.TemplateID, step.TemplateVersion)
		} else {
			for _, frozen := range frozenIR.Steps {
				if frozen.Key == step.Key && frozen.Template.ID == step.TemplateID && frozen.Template.Version == step.TemplateVersion {
					tpl = &wfdomain.TemplateSnapshot{ID: frozen.Template.ID, OutputAssetKind: frozen.OutputAssetKind, Platform: frozen.Capability.Provider, GenWay: frozen.Capability.GenerationMethod, GenType: frozen.Capability.GenerationType, NeedsConfirm: frozen.ApprovalRequired, CandidateCount: frozen.CandidateCount}
					break
				}
			}
			if tpl == nil {
				err = errors.New("step missing from frozen compilation")
			}
		}
		if err != nil {
			return nil, fmt.Errorf("template %s v%d: %w", step.TemplateID, step.TemplateVersion, err)
		}
		sr := &domain.StepRun{
			ID: "sr_" + uuid.NewString(), JobID: jobID,
			StepKey: step.Key, StepOrder: step.Order,
			TemplateID: step.TemplateID, TemplateVersion: step.TemplateVersion,
			ApprovalPolicy: approvalPolicyFromWorkflow(step.ApprovalPolicy),
			Status:         domain.SRBlocked, ResolvedInputs: map[string]any{}, Candidates: []domain.Candidate{},
			Platform: tpl.Platform, GenWay: tpl.GenWay, GenType: tpl.GenType,
			AssetKind: tpl.OutputAssetKind, NeedsConfirm: tpl.NeedsConfirm, CandidateCount: tpl.CandidateCount,
		}
		stepRuns = append(stepRuns, sr)
	}
	return s.enqueueInitialStart(ctx, job, stepRuns, compilation, submission)
}

func taskDomainsForIR(ir executable.ExecutableWorkflowIR, submittedBy string) (map[string]string, error) {
	taskToDomain := make(map[string]string, len(ir.Steps))
	for _, step := range ir.Steps {
		taskDef := strings.TrimSpace(step.Capability.ConductorTaskDef)
		if taskDef == "" {
			continue
		}
		target := capabilitydomain.ExecutionTarget(step.Capability.ExecutionTarget)
		// IR v1/v2 predate execution_target and only supported Personal Worker.
		if target == "" && ir.SchemaVersion < 3 {
			target = capabilitydomain.ExecutionTargetPersonalWorker
		}
		var executionDomain string
		switch target {
		case capabilitydomain.ExecutionTargetPersonalWorker:
			if strings.TrimSpace(step.Capability.ManagedPoolKey) != "" {
				return nil, fmt.Errorf("capability %s@%d personal target cannot declare managed pool", step.Capability.Key, step.Capability.Version)
			}
			executionDomain = domain.ExecutionDomainForUser(submittedBy)
		case capabilitydomain.ExecutionTargetManagedWorker:
			poolKey := strings.TrimSpace(step.Capability.ManagedPoolKey)
			if poolKey == "" {
				return nil, fmt.Errorf("capability %s@%d managed target requires managed pool", step.Capability.Key, step.Capability.Version)
			}
			executionDomain = domain.ExecutionDomainForManagedPool(poolKey)
		default:
			return nil, fmt.Errorf("capability %s@%d has invalid execution target %q", step.Capability.Key, step.Capability.Version, target)
		}
		if existing, ok := taskToDomain[taskDef]; ok && existing != executionDomain {
			return nil, fmt.Errorf("conductor task definition %q maps to conflicting execution domains", taskDef)
		}
		taskToDomain[taskDef] = executionDomain
	}
	return taskToDomain, nil
}

// validateBundle 闸2: bundle vs input_schema。
// 校验（按 docs/ddd/media_workflow/workflow/0002-workflow_execution/workflow_execution.md §1 + impl.md §4.2）：
//   - required 全覆盖（含 PRODUCT/VIRTUAL_IP 非空）
//   - material[]/asset[] name 唯一
//   - 无多余类型：bundle 中每个 material/asset name 必须能在 input_schema 中找到同 resource_type 的 entry
func (s *Service) validateBundle(schema []wfdomain.InputSchemaEntry, input domain.JobInput) error {
	// 1. name 唯一校验（material / asset 各自 name 唯一）
	matSeen := make(map[string]bool)
	for _, m := range input.Materials {
		if matSeen[m.Name] {
			return fmt.Errorf("duplicate material name in bundle: %s", m.Name)
		}
		matSeen[m.Name] = true
	}
	assetSeen := make(map[string]bool)
	for _, a := range input.Assets {
		if assetSeen[a.Name] {
			return fmt.Errorf("duplicate asset name in bundle: %s", a.Name)
		}
		assetSeen[a.Name] = true
	}
	// 2. input_schema 索引（用于 required、类型和 asset_kind 校验）。
	schemaByName := make(map[string]wfdomain.InputSchemaEntry, len(schema))
	for _, e := range schema {
		schemaByName[e.Name] = e
	}
	// 3. required 覆盖校验
	for _, e := range schema {
		if !e.Required {
			continue
		}
		switch e.ResourceType {
		case "MATERIAL":
			if !matSeen[e.Name] {
				return fmt.Errorf("missing required material input: %s", e.Name)
			}
		case "ASSET":
			if !assetSeen[e.Name] {
				return fmt.Errorf("missing required asset input: %s", e.Name)
			}
		case "PRODUCT":
			if input.Product == nil {
				return fmt.Errorf("missing required product input")
			}
		case "VIRTUAL_IP":
			if input.VirtualIP == nil {
				return fmt.Errorf("missing required virtual_ip input")
			}
		}
	}
	// 4. 无多余类型校验：bundle 中每个 material/asset name 必须在 input_schema 中存在且 resource_type 匹配
	for name := range matSeen {
		entry, ok := schemaByName[name]
		if !ok {
			return fmt.Errorf("material %q not declared in workflow input_schema (extra type)", name)
		}
		if entry.ResourceType != "MATERIAL" {
			return fmt.Errorf("material %q resource_type mismatch: schema=%s, bundle=MATERIAL", name, entry.ResourceType)
		}
		for _, material := range input.Materials {
			if material.Name == name && entry.AssetKind != "" && material.AssetKind != entry.AssetKind {
				return fmt.Errorf("material %q asset_kind mismatch: schema=%s, bundle=%s", name, entry.AssetKind, material.AssetKind)
			}
		}
	}
	for name := range assetSeen {
		entry, ok := schemaByName[name]
		if !ok {
			return fmt.Errorf("asset %q not declared in workflow input_schema (extra type)", name)
		}
		if entry.ResourceType != "ASSET" {
			return fmt.Errorf("asset %q resource_type mismatch: schema=%s, bundle=ASSET", name, entry.ResourceType)
		}
		for _, asset := range input.Assets {
			if asset.Name == name && entry.AssetKind != "" && asset.AssetKind != entry.AssetKind {
				return fmt.Errorf("asset %q asset_kind mismatch: schema=%s, bundle=%s", name, entry.AssetKind, asset.AssetKind)
			}
		}
	}
	return nil
}

func approvalPolicyFromWorkflow(policy wfdomain.ApprovalPolicy) domain.ApprovalPolicy {
	if policy == wfdomain.ApprovalAlways {
		return domain.ApprovalAlways
	}
	return domain.ApprovalWhenMultipleCandidates
}

// —— CancelJob ——
// Native cancellation persists intent before asynchronous engine termination.
// Cached failed/succeeded state cannot veto an active Conductor execution.
func (s *Service) Cancel(ctx context.Context, jobID string) (*domain.Job, error) {
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status == domain.JobCancelled {
		return job, nil
	}
	if job.Origin == "native" && job.ConductorWorkflowID != "" {
		if s.engine == nil {
			return nil, errors.New("conductor cancellation unavailable")
		}
		execution, err := s.engine.Get(ctx, job.ConductorWorkflowID)
		if err != nil || execution == nil {
			return nil, errors.New("conductor cancellation state unavailable")
		}
		if execution.WorkflowInstanceID != "" && execution.WorkflowInstanceID != job.ConductorWorkflowID {
			return nil, errors.New("conductor cancellation identity conflict")
		}
		if execution.Status != "RUNNING" && execution.Status != "PAUSED" {
			return nil, errors.New("conductor execution is not active; cannot cancel")
		}
	}
	if cancellations, ok := s.starts.(domain.ExecutionStartCancellationRepo); ok {
		handled, err := cancellations.CancelInitialStart(ctx, jobID)
		if err != nil {
			return nil, errors.New("workflow cancellation was not persisted")
		}
		if handled {
			return s.jobRepo.Get(ctx, jobID)
		}
	}
	if job.Origin == "native" {
		return nil, errors.New("native cancellation command unavailable")
	}
	if job.Status.IsTerminal() {
		return nil, fmt.Errorf("job %s status %s is terminal, cannot cancel", jobID, job.Status)
	}
	if job.ConductorWorkflowID != "" && s.engine != nil {
		if err := s.engine.Terminate(ctx, job.ConductorWorkflowID, "cancelled by user"); err != nil {
			return nil, fmt.Errorf("terminate conductor workflow: %w", err)
		}
	}
	if err := s.jobRepo.UpdateStatus(ctx, jobID, domain.JobCancelled, "cancelled by user"); err != nil {
		return nil, errors.New("workflow cancellation was not persisted")
	}
	return s.jobRepo.Get(ctx, jobID)
}

// Retry continues the existing Conductor execution. An input override retains
// the explicit legacy rerun contract; it is not a failure-continuation retry.
func (s *Service) Retry(ctx context.Context, jobID string, reason string, inputOverride *domain.JobInput) (*domain.Job, error) {
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Origin == "legacy_import" {
		return nil, errors.New("historical execution cannot use ordinary Retry; use the resume recovery path")
	}
	if inputOverride == nil {
		return s.continueExecution(ctx, job, reason)
	}
	if job.ConductorWorkflowID == "" || s.engine == nil {
		return nil, errors.New("conductor execution unavailable")
	}
	if s.tx == nil {
		return nil, errors.New("rerun transaction unavailable")
	}
	if job.Status == domain.JobCancelled {
		return nil, errors.New("cancelled execution cannot change input")
	}
	before, err := s.engine.Get(ctx, job.ConductorWorkflowID)
	if err != nil || before == nil || before.WorkflowInstanceID != job.ConductorWorkflowID {
		return nil, errors.New("conductor rerun state unavailable or conflicting")
	}
	if before.Status != "FAILED" && before.Status != "TIMED_OUT" {
		return nil, errors.New("input override rerun requires a failed or timed out native execution")
	}
	frozen, err := s.freezeRetryInput(ctx, job, *inputOverride)
	if err != nil {
		return nil, err
	}
	expected := *job
	input := taskcontext.PreserveTrace(before.Input, map[string]any{
		"job_id": job.ID, "inputs": conductorInputs(frozen),
	})
	submitErr := s.engine.RerunWorkflow(ctx, job.ConductorWorkflowID, input)
	observed, err := s.engine.Get(ctx, job.ConductorWorkflowID)
	if err != nil || !confirmedRerun(before, observed, input, submitErr != nil) {
		return nil, errors.New("conductor rerun outcome unconfirmed; query existing execution before retrying")
	}
	if err := s.commitRerunProjection(ctx, expected, reason, &frozen); err != nil {
		return nil, err
	}
	updated, _, err := s.Get(ctx, jobID)
	return updated, err
}

// RetryStepRun is a convenient entry point to the parent execution's native
// retry. Conductor, not the selected business node, owns the recovery scope.
func (s *Service) RetryStepRun(ctx context.Context, stepRunID, reason string, inputOverride *domain.JobInput) (*domain.StepRun, error) {
	sr, err := s.stepRunRepo.Get(ctx, stepRunID)
	if err != nil {
		return nil, err
	}
	if inputOverride != nil {
		return nil, errors.New("step retry cannot change input; use the whole-job input override rerun")
	}
	if _, err := s.Retry(ctx, sr.JobID, reason, nil); err != nil {
		return nil, err
	}
	return s.GetStepRun(ctx, stepRunID)
}

// GetStepRun refreshes the parent execution projection before returning a step.
func (s *Service) GetStepRun(ctx context.Context, stepRunID string) (*domain.StepRun, error) {
	sr, err := s.stepRunRepo.Get(ctx, stepRunID)
	if err != nil {
		return nil, err
	}
	_, steps, err := s.Get(ctx, sr.JobID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if step.ID == stepRunID {
			return step, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Job, []*domain.StepRun, error) {
	job, err := s.jobRepo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if job.ConductorWorkflowID != "" && s.engine != nil {
		if err := s.reconcileConductor(ctx, job); err != nil && !errors.Is(err, errExecutionSnapshotStale) {
			return nil, nil, fmt.Errorf("reconcile conductor execution: %w", err)
		}
	}
	job, steps, err := s.jobRepo.GetWithStepRuns(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if err := s.annotateReviews(ctx, id, steps); err != nil {
		return nil, nil, err
	}
	steps, err = s.candidateViews(ctx, job, steps)
	if err != nil {
		return nil, nil, err
	}
	return job, steps, nil
}

// Authorize the stored record before Get can reconcile Conductor state.
// A delegated organization cannot access older Jobs with no tenant attribution.
func (s *Service) GetForUser(ctx context.Context, id, userID, organizationID string) (*domain.Job, []*domain.StepRun, error) {
	if userID == "" {
		return nil, nil, gorm.ErrRecordNotFound
	}
	j, err := s.jobRepo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if j.SubmittedBy != userID || (organizationID != "" && j.OrganizationID != organizationID) {
		return nil, nil, gorm.ErrRecordNotFound
	}
	if principal, ok := internaljwt.AgentPrincipalFromContext(ctx); ok {
		if principal.UserID != userID || principal.OrganizationID != j.OrganizationID || principal.WorkspaceID != j.WorkspaceID {
			return nil, nil, gorm.ErrRecordNotFound
		}
		if s.resumeAccess == nil {
			return nil, nil, ErrSubmitAuthorityUnavailable
		}
		allowed, err := s.resumeAccess.CanUseWorkflow(ctx, j.WorkspaceID, userID)
		if err != nil {
			return nil, nil, ErrSubmitAuthorityUnavailable
		}
		if !allowed {
			return nil, nil, gorm.ErrRecordNotFound
		}
	}
	return s.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, q domain.ListQuery) ([]*domain.Job, int, error) {
	if p, ok := internaljwt.AgentPrincipalFromContext(ctx); ok {
		if s.resumeAccess == nil {
			return nil, 0, ErrSubmitAuthorityUnavailable
		}
		allowed, err := s.resumeAccess.CanUseWorkflow(ctx, p.WorkspaceID, p.UserID)
		if err != nil {
			return nil, 0, ErrSubmitAuthorityUnavailable
		}
		if !allowed {
			return nil, 0, ErrSubmitForbidden
		}
		q.SubmittedBy = p.UserID
		q.OrganizationID = p.OrganizationID
		q.WorkspaceID = p.WorkspaceID
	}
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	items, total, err := s.jobRepo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	s.refreshListedExecutions(ctx, items)
	return items, total, nil
}

// Each Job owns its projection transaction. Independent reads can overlap,
// but a page must not fan out one goroutine per row or reorder its results.
func (s *Service) refreshListedExecutions(ctx context.Context, items []*domain.Job) {
	if s.engine == nil || len(items) == 0 {
		return
	}
	const concurrency = 4
	indices := make(chan int)
	var workers sync.WaitGroup
	for range min(concurrency, len(items)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range indices {
				if ctx.Err() != nil {
					continue
				}
				job := items[index]
				if job.ConductorWorkflowID == "" {
					continue
				}
				if err := s.reconcileConductor(ctx, job); err != nil && !errors.Is(err, errExecutionSnapshotStale) {
					continue
				}
				if updated, err := s.jobRepo.Get(ctx, job.ID); err == nil {
					items[index] = updated
				}
			}
		}()
	}
dispatch:
	for index := range items {
		select {
		case <-ctx.Done():
			break dispatch
		case indices <- index:
		}
	}
	close(indices)
	workers.Wait()
}

func conductorInputs(input domain.JobInput) map[string]any {
	result := make(map[string]any)
	if input.Product != nil {
		result["product"] = map[string]any{"value": input.Product, "asset_ref": input.Product.ImageURL}
	}
	if input.VirtualIP != nil {
		result["virtual_ip"] = map[string]any{"value": input.VirtualIP, "asset_ref": input.VirtualIP.FullBodyURL}
	}
	for _, asset := range input.Assets {
		result[asset.Name] = map[string]any{"asset_ref": asset.URL, "url": asset.URL, "content": asset.Content, "asset_kind": asset.AssetKind}
	}
	for _, material := range input.Materials {
		result[material.Name] = map[string]any{"asset_ref": material.URL, "url": material.URL, "content": material.Content, "asset_kind": material.AssetKind}
	}
	return result
}

func (s *Service) reconcileConductor(ctx context.Context, job *domain.Job) error {
	ctx = context.WithValue(ctx, executionSnapshotKey{}, executionSnapshot{jobID: job.ID, engineID: job.ConductorWorkflowID, retryCount: job.RetryCount})
	execution, err := s.engine.Get(ctx, job.ConductorWorkflowID)
	if err != nil {
		return err
	}
	if execution == nil {
		return errors.New("execution snapshot unavailable")
	}
	current, err := s.jobRepo.Get(ctx, job.ID)
	if err != nil {
		return err
	}
	if current.Status == domain.JobCancelled {
		return nil
	}
	if err := checkExecutionSnapshot(ctx, current); err != nil {
		return err
	}
	stepRuns, err := s.stepRunRepo.ListByJob(ctx, job.ID)
	if err != nil {
		return err
	}
	stepRunByKey := make(map[string]*domain.StepRun, len(stepRuns))
	generationSnapshots := make(map[string]generationSnapshot, len(stepRuns))
	for _, stepRun := range stepRuns {
		stepRunByKey[stepRun.StepKey] = stepRun
		generationSnapshots[stepRun.ID] = makeGenerationSnapshot(job.ConductorWorkflowID, stepRun, execution)
	}
	ctx = context.WithValue(ctx, generationSnapshotsKey{}, generationSnapshots)
	latest := latestGeneration(execution)
	tasks := append([]domain.EngineTask(nil), execution.Tasks...)
	phaseOrder := map[string]int{"render": 0, "invoke": 1, "approval": 2, "finalize": 3}
	sort.SliceStable(tasks, func(i, j int) bool {
		_, a, _ := parseTaskReference(tasks[i].ReferenceTaskName)
		_, b, _ := parseTaskReference(tasks[j].ReferenceTaskName)
		return phaseOrder[a] < phaseOrder[b]
	})
	waitingApproval := false
	for _, task := range tasks {
		if task.Retried || !currentGeneration(task, latest) {
			continue
		}
		stepKey, phase, ok := parseTaskReference(task.ReferenceTaskName)
		if !ok {
			continue
		}
		stepRun := stepRunByKey[stepKey]
		if stepRun == nil {
			continue
		}
		// withActiveStepProjection already preserves these completed outputs.
		// Avoid repeating its lock/read transactions for render/invoke phases
		// that cannot change the projection. The engine is still read on every
		// request and execution/review/finalize guards remain unchanged.
		if (phase == "render" || phase == "invoke") && stepRun.Status == domain.SRSucceeded && stepRun.OutputAssetID != "" {
			continue
		}
		switch phase {
		case "render":
			if prompt, ok := task.Output["prompt"].(string); ok && prompt != "" {
				if err := s.withActiveStepProjection(ctx, job.ID, stepRun.ID, func(txCtx context.Context) error {
					return s.stepRunRepo.UpdateResolvedInputs(txCtx, stepRun.ID, task.Input, prompt)
				}); err != nil {
					return err
				}
			}
		case "invoke":
			if err := s.withActiveStepProjection(ctx, job.ID, stepRun.ID, func(ctx context.Context) error {
				if task.Status == "IN_PROGRESS" || task.Status == "SCHEDULED" {
					if err := s.stepRunRepo.UpdateCandidates(ctx, stepRun.ID, []domain.Candidate{}); err != nil {
						return err
					}
					if err := s.stepRunRepo.UpdateStatus(ctx, stepRun.ID, domain.SRClaimed, map[string]any{"selected_index": nil, "retry_count": task.RetryCount, "failure_reason": "", "finished_at": nil}); err != nil {
						return err
					}
				}
				if rawCandidates, ok := task.Output["candidates"]; ok {
					var candidates []domain.Candidate
					encoded, err := json.Marshal(rawCandidates)
					if err != nil || json.Unmarshal(encoded, &candidates) != nil {
						return errors.New("invalid capability candidates")
					}
					if err := s.stepRunRepo.UpdateCandidates(ctx, stepRun.ID, candidates); err != nil {
						return err
					}
				}
				if task.Status == "FAILED" || task.Status == "FAILED_WITH_TERMINAL_ERROR" || task.Status == "TIMED_OUT" {
					if err := s.stepRunRepo.UpdateStatus(ctx, stepRun.ID, domain.SRFailed, nil); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
		case "approval":
			if s.reviews != nil {
				if task.Status == "COMPLETED" {
					if err := s.projectCompletedReview(ctx, job.ID, stepRun.ID, task); err != nil {
						return err
					}
				}
				if task.Status == "IN_PROGRESS" || task.Status == "SCHEDULED" {
					if err := s.projectWaitingReview(ctx, job.ID, stepRun.ID, task); err != nil {
						return err
					}
				}
				continue
			}
			if task.Status == "IN_PROGRESS" || task.Status == "SCHEDULED" {
				waitingApproval = true
			}
			if rawCandidates, ok := task.Input["candidates"]; ok && (task.Status == "IN_PROGRESS" || task.Status == "SCHEDULED") {
				var candidates []domain.Candidate
				encoded, err := json.Marshal(rawCandidates)
				if err != nil || json.Unmarshal(encoded, &candidates) != nil {
					return errors.New("invalid approval candidates")
				}
				if err := s.stepRunRepo.UpdateCandidates(ctx, stepRun.ID, candidates); err != nil {
					return err
				}
			}
			if task.Status == "IN_PROGRESS" || task.Status == "SCHEDULED" {
				if err := s.stepRunRepo.UpdateStatus(ctx, stepRun.ID, domain.SRNeedConfirm, nil); err != nil {
					return err
				}
			}
		case "finalize":
			if task.Status == "COMPLETED" && (stepRun.Status != domain.SRSucceeded || strings.TrimSpace(stepRun.OutputAssetID) == "") {
				resource, _ := task.Output["output_resource"].(map[string]any)
				id, _ := task.Output["output_resource_id"].(string)
				resourceID, _ := resource["id"].(string)
				if !strings.HasPrefix(id, "gen_") || resourceID != id {
					return errors.New("finalize returned no registered asset identity")
				}
				if err := s.withExecutionProjection(ctx, job.ID, stepRun.ID, func(txCtx context.Context, current *domain.Job) error {
					if current.Status == domain.JobCancelled {
						return nil
					}
					fresh, err := s.stepRunRepo.Get(txCtx, stepRun.ID)
					if err != nil {
						return err
					}
					if fresh.Status == domain.SRSucceeded && fresh.OutputAssetID != "" {
						if fresh.OutputAssetID != id {
							return errors.New("finalize asset identity conflicts with projected output")
						}
						return nil
					}
					if err := s.stepRunRepo.UpdateOutput(txCtx, stepRun.ID, id); err != nil {
						return err
					}
					return s.stepRunRepo.UpdateStatus(txCtx, stepRun.ID, domain.SRSucceeded, nil)
				}); err != nil {
					return err
				}
			} else if task.Status == "FAILED" || task.Status == "FAILED_WITH_TERMINAL_ERROR" || task.Status == "TIMED_OUT" {
				if err := s.withActiveStepProjection(ctx, job.ID, stepRun.ID, func(txCtx context.Context) error {
					return s.stepRunRepo.UpdateStatus(txCtx, stepRun.ID, domain.SRFailed, nil)
				}); err != nil {
					return err
				}
			}
		}
	}
	switch execution.Status {
	case "COMPLETED":
		return s.withExecutionProjection(ctx, job.ID, "", func(txCtx context.Context, current *domain.Job) error {
			if current.Status == domain.JobCancelled {
				return nil
			}
			steps, err := s.stepRunRepo.ListByJob(txCtx, job.ID)
			if err != nil {
				return err
			}
			if len(steps) == 0 {
				return errors.New("completed execution has no persisted steps")
			}
			for _, step := range steps {
				if step.Status != domain.SRSucceeded || strings.TrimSpace(step.OutputAssetID) == "" {
					return errors.New("completed execution has unpersisted step outputs")
				}
			}
			if current.Status == domain.JobSucceeded && current.FailureReason == "" {
				return nil
			}
			return s.jobRepo.UpdateStatus(txCtx, job.ID, domain.JobSucceeded, "")
		})
	case "FAILED", "FAILED_WITH_TERMINAL_ERROR", "TIMED_OUT":
		return s.projectExecutionStatus(ctx, job.ID, domain.JobFailed, "conductor workflow "+execution.Status)
	case "TERMINATED":
		return s.projectExecutionStatus(ctx, job.ID, domain.JobCancelled, "conductor workflow TERMINATED")
	case "PAUSED":
		return s.projectExecutionStatus(ctx, job.ID, domain.JobPaused, "")
	default:
		if execution.Status == "RUNNING" {
			if s.reviews != nil {
				return s.refreshReviewJobStatus(ctx, job.ID)
			}
			status := domain.JobRunning
			if waitingApproval {
				status = domain.JobNeedConfirm
			}
			if job.Status != status {
				return s.projectExecutionStatus(ctx, job.ID, status, "")
			}
		}
		return nil
	}
}

func parseTaskReference(reference string) (string, string, bool) {
	if key, phase, _, ok := taskref.Parse(reference); ok {
		return key, phase, true
	}
	const prefix = "step__"
	if !strings.HasPrefix(reference, prefix) {
		return "", "", false
	}
	remainder := strings.TrimPrefix(reference, prefix)
	for _, phase := range []string{"render", "invoke", "approval", "finalize"} {
		suffix := "__" + phase
		if strings.HasSuffix(remainder, suffix) {
			return strings.TrimSuffix(remainder, suffix), phase, true
		}
	}
	return "", "", false
}

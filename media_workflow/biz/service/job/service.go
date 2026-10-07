package job

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"
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
	"media_agent/media_workflow/biz/shared/taskref"
	"media_agent/media_workflow/biz/workflow/executable"
)

// GeneratedAssetSaver 落产物端口（由 client.MaterialsClient 实现，main.go 注入）。
type GeneratedAssetSaver interface {
	UpsertGeneratedAsset(ctx context.Context, req *client.UpsertGenAssetReq) (string, error)
}

// Service job 用例编排（pull 模式 + NEED_CONFIRM）。
type Service struct {
	jobRepo             domain.JobRepo
	stepRunRepo         domain.StepRunRepo
	tx                  dal.TxRunner
	wfRepo              domain.WorkflowLookup
	tplLookup           wfdomain.TemplateLookup
	materials           GeneratedAssetSaver
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
		return nil, ErrSubmitForbidden
	}
	// workflow_version is optional at the HTTP boundary. Zero means "use the
	// current published version"; a non-zero value remains an optimistic lock.
	if cmd.WorkflowVersion != 0 && wf.Version != cmd.WorkflowVersion {
		return nil, fmt.Errorf("workflow version mismatch: want %d, got %d", wf.Version, cmd.WorkflowVersion)
	}
	conductorMode := s.compilations != nil && s.engine != nil
	var compilation *wfdomain.Compilation
	if conductorMode {
		if wf.Status != wfdomain.StatusPublished {
			return nil, fmt.Errorf("workflow %s is not published", wf.ID)
		}
		compilation, err = s.compilations.GetRegistered(ctx, wf.ID, wf.Version)
		if err != nil {
			return nil, fmt.Errorf("registered workflow compilation %s@%d: %w", wf.ID, wf.Version, err)
		}
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
	for _, step := range wf.Steps {
		// 判断 root（无 STEP_OUTPUT 依赖）→ PENDING；否则 BLOCKED
		isRoot := true
		for _, b := range step.SlotBindings {
			if b.Source == wfdomain.SourceStepOutput {
				isRoot = false
				break
			}
		}
		status := domain.SRBlocked
		if isRoot && !conductorMode {
			status = domain.SRPending
		}
		// 加载 template 快照（反范式化 platform/gen_way/asset_kind 等）
		tpl, err := s.tplLookup.GetVersion(ctx, step.TemplateID, step.TemplateVersion)
		if err != nil {
			return nil, fmt.Errorf("template %s v%d: %w", step.TemplateID, step.TemplateVersion, err)
		}
		sr := &domain.StepRun{
			ID: "sr_" + uuid.NewString(), JobID: jobID,
			StepKey: step.Key, StepOrder: step.Order,
			TemplateID: step.TemplateID, TemplateVersion: step.TemplateVersion,
			ApprovalPolicy: approvalPolicyFromWorkflow(step.ApprovalPolicy),
			Status:         status, ResolvedInputs: map[string]any{}, Candidates: []domain.Candidate{},
			Platform: tpl.Platform, GenWay: tpl.GenWay, GenType: tpl.GenType,
			AssetKind: tpl.OutputAssetKind, NeedsConfirm: tpl.NeedsConfirm, CandidateCount: tpl.CandidateCount,
		}
		stepRuns = append(stepRuns, sr)
	}
	if conductorMode {
		return s.enqueueInitialStart(ctx, job, stepRuns, compilation, submission)
	}
	if err := s.jobRepo.Save(ctx, job, stepRuns); err != nil {
		return nil, fmt.Errorf("save job: %w", err)
	}
	if err := s.jobRepo.UpdateStatus(ctx, jobID, domain.JobRunning, ""); err != nil {
		return nil, err
	}
	return s.jobRepo.Get(ctx, jobID)
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

// —— ClaimStepRun ——
type ClaimQuery = domain.ClaimQuery

func (s *Service) Claim(ctx context.Context, q ClaimQuery) (*ClaimResult, error) {
	sr, err := s.stepRunRepo.ClaimAtomic(ctx, q)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no claimable step_run")
		}
		return nil, err
	}
	// 加载 job（取 bundle）
	job, err := s.jobRepo.Get(ctx, sr.JobID)
	if err != nil {
		return nil, err
	}
	// 加载 template（取 prompt_body + slots）
	tpl, err := s.tplLookup.GetVersion(ctx, sr.TemplateID, sr.TemplateVersion)
	if err != nil {
		return nil, err
	}
	// 解析 slot 值 + 渲染 prompt
	resolved, prompt, assets := s.resolveAndRender(ctx, sr, tpl, job.Input)
	_ = s.stepRunRepo.UpdateResolvedInputs(ctx, sr.ID, resolved, prompt)
	return &ClaimResult{
		StepRunID: sr.ID, JobID: sr.JobID, RenderedPrompt: prompt,
		InputAssets: assets, Platform: sr.Platform, GenWay: sr.GenWay,
		GenType: sr.GenType, AssetKind: sr.AssetKind, CandidateCount: sr.CandidateCount,
	}, nil
}

type ClaimResult struct {
	StepRunID      string
	JobID          string
	RenderedPrompt string
	InputAssets    []ClaimAsset
	Platform       string
	GenWay         string
	GenType        string
	AssetKind      string
	CandidateCount int
}
type ClaimAsset struct{ Label, URL, Content string }

// resolveAndRender 解析 binding -> resolved slot 值 + 用 Go template 渲染 prompt。
//
// prompt_body 用 Go template 语法 {{.name.field}}，通用引用 bundle 任意字段：
//   - {{.virtual_ip.full_body_url}} / {{.virtual_ip.avatar_url}} / {{.virtual_ip.three_view_url}} ...
//   - {{.product.target_audience}} / {{.product.product_details}} ...
//   - {{.scene_image.url}} / {{.scene_image.content}}（material/asset 按 slot.name 索引）
//   - {{.prev_step.url}}（STEP_OUTPUT 前驱产出）
//   - {{.const_slot}}（CONST 字面值）
//
// bundle 序列化成 map[string]any（json tag 字段名），Go template 自动按属性查找；
// 加新素材类型/字段不需改渲染（bundle 序列化自动反映）。
func (s *Service) resolveAndRender(ctx context.Context, sr *domain.StepRun, tpl *wfdomain.TemplateSnapshot, input domain.JobInput) (map[string]any, string, []ClaimAsset) {
	resolved := map[string]any{}
	assets := []ClaimAsset{}
	data := map[string]any{}

	// 1. bundle -> data（Go template 友好：json 序列化后字段按 json tag 小写）
	if input.Product != nil {
		data["product"] = toJSONMap(input.Product)
	}
	if input.VirtualIP != nil {
		data["virtual_ip"] = toJSONMap(input.VirtualIP)
	}
	for _, m := range input.Materials {
		data[m.Name] = toJSONMap(m)
	}
	for _, a := range input.Assets {
		data[a.Name] = toJSONMap(a)
	}

	// 2. 加载 workflow + bindings + step_runs（STEP_OUTPUT 解析用）
	job, err := s.jobRepo.Get(ctx, sr.JobID)
	if err != nil {
		return resolved, renderTemplateSafe(tpl.PromptBody, data), assets
	}
	wf, err := s.wfRepo.Get(ctx, job.WorkflowID)
	if err != nil {
		return resolved, renderTemplateSafe(tpl.PromptBody, data), assets
	}
	var bindings []wfdomain.SlotBinding
	for _, step := range wf.Steps {
		if step.Key == sr.StepKey {
			bindings = step.SlotBindings
			break
		}
	}
	allSRs, _ := s.stepRunRepo.ListByJob(ctx, sr.JobID)
	srByKey := make(map[string]*domain.StepRun, len(allSRs))
	for _, s2 := range allSRs {
		srByKey[s2.StepKey] = s2
	}

	// 3. 解析 binding：STEP_OUTPUT/CONST 填 data + resolved + assets；WORKFLOW_INPUT 从 data 取填 resolved
	for _, b := range bindings {
		switch b.Source {
		case wfdomain.SourceWorkflowInput:
			if slotData, ok := data[b.Ref].(map[string]any); ok {
				resolved[b.ToInput] = slotData
				url, _ := slotData["url"].(string)
				content, _ := slotData["content"].(string)
				assets = append(assets, ClaimAsset{Label: b.ToInput, URL: url, Content: content})
			}
		case wfdomain.SourceStepOutput:
			if prev, ok := srByKey[b.Ref]; ok && prev.SelectedIndex != nil && *prev.SelectedIndex < len(prev.Candidates) {
				c := prev.Candidates[*prev.SelectedIndex]
				slotData := map[string]any{"url": c.URL, "content": c.Content}
				data[b.ToInput] = slotData
				resolved[b.ToInput] = slotData
				assets = append(assets, ClaimAsset{Label: b.ToInput, URL: c.URL, Content: c.Content})
			}
		case wfdomain.SourceConst:
			data[b.ToInput] = b.ConstValue
			resolved[b.ToInput] = map[string]any{"value": b.ConstValue}
			assets = append(assets, ClaimAsset{Label: b.ToInput, Content: b.ConstValue})
		}
	}

	// 4. Go template 渲染（parse/execute 失败回退原文）
	prompt := renderTemplateSafe(tpl.PromptBody, data)
	return resolved, prompt, assets
}

// renderTemplateSafe 用 Go template 渲染 prompt（parse/execute 失败则回退原文，不阻塞 claim）。
func renderTemplateSafe(promptBody string, data map[string]any) string {
	tmpl, err := template.New("prompt").Parse(promptBody)
	if err != nil {
		return promptBody
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return promptBody
	}
	return buf.String()
}

// toJSONMap 把 struct 值转成 map[string]any（Go template 友好，字段按 json tag 小写）。
func toJSONMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}

// —— CompleteStepRun ——
func (s *Service) Complete(ctx context.Context, stepRunID string, candidates []domain.Candidate) (*domain.StepRun, error) {
	sr, err := s.stepRunRepo.Get(ctx, stepRunID)
	if err != nil {
		return nil, err
	}
	if sr.Status != domain.SRClaimed {
		return nil, fmt.Errorf("step_run %s status %s, expected claimed", stepRunID, sr.Status)
	}
	if len(candidates) == 0 {
		return nil, errors.New("no candidates provided")
	}
	_ = s.stepRunRepo.UpdateCandidates(ctx, stepRunID, candidates)
	needsApproval := domain.NeedsApproval(sr.ApprovalPolicy, len(candidates))
	if sr.NeedsConfirm || needsApproval {
		// → NEED_CONFIRM
		_ = s.stepRunRepo.UpdateStatus(ctx, stepRunID, domain.SRNeedConfirm, nil)
		_ = s.jobRepo.UpdateStatus(ctx, sr.JobID, domain.JobNeedConfirm, "")
	} else {
		// 直接 SUCCEEDED：取 primary candidate（无 primary 标记则取第 0 个）
		primaryIdx := 0
		primary := candidates[0]
		for i, c := range candidates {
			if c.IsPrimary {
				primaryIdx = i
				primary = c
				break
			}
		}
		if err := s.finalizeStepRun(ctx, sr, primary, &primaryIdx, ""); err != nil {
			return nil, err
		}
	}
	return s.stepRunRepo.Get(ctx, stepRunID)
}

func approvalPolicyFromWorkflow(policy wfdomain.ApprovalPolicy) domain.ApprovalPolicy {
	if policy == wfdomain.ApprovalAlways {
		return domain.ApprovalAlways
	}
	return domain.ApprovalWhenMultipleCandidates
}

// —— ConfirmStepRun ——
func (s *Service) Confirm(ctx context.Context, stepRunID string, selectedIndex int) (*domain.StepRun, error) {
	sr, err := s.stepRunRepo.Get(ctx, stepRunID)
	if err != nil {
		return nil, err
	}
	if sr.Status != domain.SRNeedConfirm {
		return nil, fmt.Errorf("step_run %s status %s, expected need_confirm", stepRunID, sr.Status)
	}
	if selectedIndex < 0 || selectedIndex >= len(sr.Candidates) {
		return nil, fmt.Errorf("selected_index %d out of range [0, %d)", selectedIndex, len(sr.Candidates))
	}
	selected := sr.Candidates[selectedIndex]
	job, err := s.jobRepo.Get(ctx, sr.JobID)
	if err != nil {
		return nil, err
	}
	if job.ConductorWorkflowID != "" && s.engine != nil {
		approval, _, err := s.currentApprovalTask(ctx, job, sr)
		if err != nil {
			return nil, err
		}
		// Do not trust a stale local candidate projection from an earlier round.
		var current []domain.Candidate
		encoded, err := json.Marshal(approval.Input["candidates"])
		if err != nil || json.Unmarshal(encoded, &current) != nil || selectedIndex >= len(current) || current[selectedIndex] != selected {
			return nil, errors.New("approval candidates changed; refresh the step before confirming")
		}
		if err := s.engine.CompleteTask(ctx, job.ConductorWorkflowID, approval.ReferenceTaskName, map[string]any{
			"decision": "accept", "selected_index": selectedIndex, "selected_candidate": selected,
		}); err != nil {
			return nil, fmt.Errorf("complete conductor approval task: %w", err)
		}
		_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRClaimed, map[string]any{"selected_index": selectedIndex})
		_ = s.jobRepo.UpdateStatus(ctx, sr.JobID, domain.JobRunning, "")
		return s.stepRunRepo.Get(ctx, stepRunID)
	}
	if err := s.finalizeStepRun(ctx, sr, selected, &selectedIndex, ""); err != nil {
		return nil, err
	}
	return s.stepRunRepo.Get(ctx, stepRunID)
}

// —— RegenerateStepRun ——
func (s *Service) Regenerate(ctx context.Context, stepRunID string) (*domain.StepRun, error) {
	sr, err := s.stepRunRepo.Get(ctx, stepRunID)
	if err != nil {
		return nil, err
	}
	if sr.Status != domain.SRNeedConfirm {
		return nil, fmt.Errorf("step_run %s status %s, expected need_confirm", stepRunID, sr.Status)
	}
	job, err := s.jobRepo.Get(ctx, sr.JobID)
	if err != nil {
		return nil, err
	}
	if job.ConductorWorkflowID != "" && s.engine != nil {
		approval, iteration, err := s.currentApprovalTask(ctx, job, sr)
		if err != nil {
			return nil, err
		}
		if iteration == 0 {
			return nil, errors.New("this published workflow has no regeneration loop; publish a new version")
		}
		if err := s.engine.CompleteTask(ctx, job.ConductorWorkflowID, approval.ReferenceTaskName, map[string]any{"decision": "regenerate"}); err != nil {
			return nil, fmt.Errorf("complete conductor regeneration decision: %w", err)
		}
		_ = s.stepRunRepo.UpdateCandidates(ctx, stepRunID, []domain.Candidate{})
		_ = s.stepRunRepo.UpdateStatus(ctx, stepRunID, domain.SRBlocked, map[string]any{"selected_index": nil})
		_ = s.jobRepo.UpdateStatus(ctx, sr.JobID, domain.JobRunning, "")
		return s.stepRunRepo.Get(ctx, stepRunID)
	}
	_ = s.stepRunRepo.UpdateStatus(ctx, stepRunID, domain.SRPending, map[string]any{"candidates": "[]"})
	_ = s.jobRepo.UpdateStatus(ctx, sr.JobID, domain.JobRunning, "")
	return sr, nil
}

// —— FailStepRun ——
// 按 workflow_execution §5：step_run=failed + Job=failed + advanceDAG(cascade skip 下游)。
// 让在跑的前驱 step 跑完（不中断已 claimed 前驱），仅把后续未启动 step 置 skipped。
func (s *Service) Fail(ctx context.Context, stepRunID string, reason string) (*domain.StepRun, error) {
	sr, err := s.stepRunRepo.Get(ctx, stepRunID)
	if err != nil {
		return nil, err
	}
	// 状态守卫：仅 claimed/need_confirm/pending 可 -> failed（已 succeeded/skipped/failed 不允许）
	switch sr.Status {
	case domain.SRClaimed, domain.SRNeedConfirm, domain.SRPending:
	default:
		return nil, fmt.Errorf("step_run %s status %s, expected claimed/need_confirm/pending", stepRunID, sr.Status)
	}
	_ = s.stepRunRepo.UpdateFailure(ctx, stepRunID, reason)
	_ = s.stepRunRepo.UpdateStatus(ctx, stepRunID, domain.SRFailed, nil)
	_ = s.jobRepo.UpdateStatus(ctx, sr.JobID, domain.JobFailed, reason)
	// 级联跳过下游：advanceDAG 会把依赖本 step 的 BLOCKED step_run -> SKIPPED（任一依赖 FAILED/SKIPPED）
	s.advanceDAG(ctx, sr.JobID, sr.StepKey)
	return sr, nil
}

// —— CancelJob ——
// 仅 running/pending/need_confirm 可 -> cancelled；已 succeeded/failed/cancelled 不可 cancel。
func (s *Service) Cancel(ctx context.Context, jobID string) (*domain.Job, error) {
	if cancellations, ok := s.starts.(domain.ExecutionStartCancellationRepo); ok {
		handled, err := cancellations.CancelInitialStart(ctx, jobID)
		if err != nil {
			return nil, errors.New("workflow cancellation was not persisted")
		}
		if handled {
			return s.jobRepo.Get(ctx, jobID)
		}
	}
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status.IsTerminal() {
		return nil, fmt.Errorf("job %s status %s is terminal, cannot cancel", jobID, job.Status)
	}
	if job.ConductorWorkflowID != "" && s.engine != nil {
		if err := s.engine.Terminate(ctx, job.ConductorWorkflowID, "cancelled by user"); err != nil {
			return nil, fmt.Errorf("terminate conductor workflow: %w", err)
		}
	}
	_ = s.jobRepo.UpdateStatus(ctx, jobID, domain.JobCancelled, "cancelled by user")
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
	if s.reviews != nil && s.tx == nil {
		return nil, errors.New("rerun transaction unavailable")
	}
	if job.Status != domain.JobFailed {
		return nil, errors.New("input override rerun requires a failed execution")
	}
	frozen, err := s.freezeRetryInput(ctx, job, *inputOverride)
	if err != nil {
		return nil, err
	}
	expected := *job
	if err := s.engine.RerunWorkflow(ctx, job.ConductorWorkflowID, map[string]any{
		"job_id": job.ID, "inputs": conductorInputs(frozen),
	}); err != nil {
		return nil, fmt.Errorf("rerun conductor workflow: %w", err)
	}
	if err := s.commitNativeJobRetry(ctx, expected, reason, &frozen); err != nil {
		return nil, err
	}
	return s.jobRepo.Get(ctx, jobID)
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
	if _, _, err := s.Get(ctx, sr.JobID); err != nil {
		return nil, err
	}
	sr, err = s.stepRunRepo.Get(ctx, stepRunID)
	if err != nil {
		return nil, err
	}
	if err := s.annotateReviews(ctx, sr.JobID, []*domain.StepRun{sr}); err != nil {
		return nil, err
	}
	return sr, nil
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Job, []*domain.StepRun, error) {
	job, _, err := s.jobRepo.GetWithStepRuns(ctx, id)
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
	for index, job := range items {
		if job.ConductorWorkflowID == "" || s.engine == nil {
			continue
		}
		if err := s.reconcileConductor(ctx, job); err != nil && !errors.Is(err, errExecutionSnapshotStale) {
			continue
		}
		if updated, err := s.jobRepo.Get(ctx, job.ID); err == nil {
			items[index] = updated
		}
	}
	return items, total, nil
}

// —— finalizeStepRun: 落产物 + SUCCEEDED + 推进 DAG ——
func (s *Service) finalizeStepRun(ctx context.Context, sr *domain.StepRun, selected domain.Candidate, selectedIndex *int, resultKey string) error {
	// 1. 调 materials 落 generated_asset
	job, err := s.jobRepo.Get(ctx, sr.JobID)
	if err != nil {
		return err
	}
	if job.Status == domain.JobCancelled {
		return nil
	}
	if err := checkExecutionSnapshot(ctx, job); err != nil {
		return err
	}
	if s.engine != nil && resultKey == "" {
		return errors.New("native generation result identity is required")
	}
	if err := s.checkGenerationSnapshot(ctx, job.ID, sr.ID); err != nil {
		return err
	}
	request := &client.UpsertGenAssetReq{
		Scope: job.Scope, AssetKind: sr.AssetKind,
		Name: safeName(sr.StepKey, sr.ID),
		URL:  selected.URL, Content: selected.Content,
		OwnerUserID: job.SubmittedBy, Visibility: "private",
		JobID: sr.JobID, StepRunID: sr.ID, TemplateID: sr.TemplateID, TemplateVersion: sr.TemplateVersion,
		ResultKey: resultKey,
	}
	if job.Input.Product != nil {
		request.SourceProductID = job.Input.Product.ID
	}
	if job.Input.VirtualIP != nil {
		request.SourceVirtualIPID = job.Input.VirtualIP.ID
	}
	assetID, err := s.materials.UpsertGeneratedAsset(ctx, request)
	if err != nil {
		return errors.New("generated asset persistence unavailable")
	}
	if strings.TrimSpace(assetID) == "" {
		return errors.New("generated asset persistence returned no identity")
	}
	// The external idempotent upsert must remain outside the database transaction.
	// Cancellation and concurrent finalizers serialize with the local projection.
	err = s.withExecutionProjection(ctx, sr.JobID, sr.ID, func(txCtx context.Context, current *domain.Job) error {
		if current.Status == domain.JobCancelled {
			return nil
		}
		if err := s.checkGenerationSnapshot(txCtx, sr.JobID, sr.ID); err != nil {
			return err
		}
		fresh, err := s.stepRunRepo.Get(txCtx, sr.ID)
		if err != nil {
			return err
		}
		if fresh.Status == domain.SRSucceeded && fresh.OutputAssetID != "" {
			if fresh.OutputAssetID != assetID {
				return errors.New("generated asset identity conflicts with finalized step")
			}
			return nil
		}
		if err := s.stepRunRepo.UpdateOutput(txCtx, sr.ID, assetID); err != nil {
			return err
		}
		updates := map[string]any{}
		if selectedIndex != nil {
			updates["selected_index"] = *selectedIndex
		}
		return s.stepRunRepo.UpdateStatus(txCtx, sr.ID, domain.SRSucceeded, updates)
	})
	if err != nil {
		return err
	}
	// Native execution advancement belongs exclusively to Conductor. Local step
	// completion alone is not proof that the engine has completed the workflow.
	if s.engine != nil {
		return nil
	}
	// 3. 推进 DAG: 下游 BLOCKED -> PENDING
	s.advanceDAG(ctx, sr.JobID, sr.StepKey)
	// 4. 检查 sink 是否 SUCCEEDED -> Job SUCCEEDED
	s.checkJobCompletion(ctx, sr.JobID)
	return nil
}

// advanceDAG: 检查哪些 BLOCKED step_run 的所有 STEP_OUTPUT 依赖已 SUCCEEDED → PENDING。
// 任一依赖 FAILED/SKIPPED → cascade SKIPPED。
func (s *Service) advanceDAG(ctx context.Context, jobID string, completedStepKey string) {
	blocked, err := s.stepRunRepo.ListByJobAndStatus(ctx, jobID, domain.SRBlocked)
	if err != nil || len(blocked) == 0 {
		return
	}
	// 加载 workflow 取 bindings（依赖关系）
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return
	}
	wf, err := s.wfRepo.Get(ctx, job.WorkflowID)
	if err != nil {
		return
	}
	// step_key -> step（含 bindings）
	stepByKey := make(map[string]wfdomain.Step, len(wf.Steps))
	for _, step := range wf.Steps {
		stepByKey[step.Key] = step
	}
	// 加载全部 step_run（查依赖状态）
	allSRs, _ := s.stepRunRepo.ListByJob(ctx, jobID)
	srByKey := make(map[string]*domain.StepRun, len(allSRs))
	for _, sr := range allSRs {
		srByKey[sr.StepKey] = sr
	}
	// 迭代直到一轮无变更（处理多级 cascade：A failed -> B skipped -> C skipped）
	for {
		changed := false
		for _, sr := range blocked {
			// 跳过已非 blocked 的（前一轮可能已推进）
			if srByKey[sr.StepKey].Status != domain.SRBlocked {
				continue
			}
			step, ok := stepByKey[sr.StepKey]
			if !ok {
				continue
			}
			// 收集该 step 的 STEP_OUTPUT 依赖
			allDepsSucceeded := true
			anyDepFailed := false
			for _, b := range step.SlotBindings {
				if b.Source != wfdomain.SourceStepOutput {
					continue
				}
				dep, ok := srByKey[b.Ref]
				if !ok {
					allDepsSucceeded = false
					continue
				}
				switch dep.Status {
				case domain.SRSucceeded:
					// ok
				case domain.SRFailed, domain.SRSkipped:
					anyDepFailed = true
				default:
					allDepsSucceeded = false
				}
			}
			if anyDepFailed {
				_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRSkipped, nil)
				srByKey[sr.StepKey].Status = domain.SRSkipped
				changed = true
			} else if allDepsSucceeded {
				_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRPending, nil)
				srByKey[sr.StepKey].Status = domain.SRPending
				changed = true
			}
		}
		if !changed {
			break
		}
	}
}

// checkJobCompletion: 如果没有 PENDING/CLAIMED/NEED_CONFIRM 的 step_run → Job SUCCEEDED。
func (s *Service) checkJobCompletion(ctx context.Context, jobID string) {
	all, err := s.stepRunRepo.ListByJob(ctx, jobID)
	if err != nil {
		return
	}
	allDone := true
	for _, sr := range all {
		if sr.Status != domain.SRSucceeded && sr.Status != domain.SRSkipped {
			allDone = false
			break
		}
	}
	if allDone {
		_ = s.jobRepo.UpdateStatus(ctx, jobID, domain.JobSucceeded, "")
	}
}

// safeName 生成 asset 名（取 ID 前 8 字符，不足取全部）。
func safeName(stepKey, id string) string {
	if len(id) > 8 {
		return fmt.Sprintf("%s_%s", stepKey, id[:8])
	}
	return fmt.Sprintf("%s_%s", stepKey, id)
}

var _ = strings.TrimSpace
var _ = json.Marshal

// StartClaimTimeoutScanner 后台扫描 CLAIMED 超时的 step_run → 回 PENDING。
// 每 60s 扫一次，超时阈值 10min（worker 崩溃保护）。
func (s *Service) StartClaimTimeoutScanner(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scanStaleClaimed(ctx)
		}
	}
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
			return s.jobRepo.UpdateStatus(txCtx, job.ID, domain.JobSucceeded, "")
		})
	case "FAILED", "FAILED_WITH_TERMINAL_ERROR", "TIMED_OUT", "TERMINATED":
		return s.projectExecutionStatus(ctx, job.ID, domain.JobFailed, "conductor workflow "+execution.Status)
	case "PAUSED":
		return s.projectExecutionStatus(ctx, job.ID, domain.JobNeedConfirm, "")
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

func approvalTaskReference(stepKey string) string {
	return "step__" + sanitizeTaskReference(stepKey) + "__approval"
}

func sanitizeTaskReference(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func (s *Service) scanStaleClaimed(ctx context.Context) {
	timeout := 600
	if s.jobCfg != nil && s.jobCfg.ClaimTimeoutSeconds > 0 {
		timeout = int(s.jobCfg.ClaimTimeoutSeconds)
	}
	n, err := s.stepRunRepo.ReclaimStale(ctx, timeout)
	if err != nil {
		return
	}
	if n > 0 {
		fmt.Printf("claim timeout: reclaimed %d stale step_runs\n", n)
	}
}

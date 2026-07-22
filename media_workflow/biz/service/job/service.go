package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_workflow/biz/dal"
	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared/client"
)

// GeneratedAssetSaver 落产物端口（由 client.MaterialsClient 实现，main.go 注入）。
type GeneratedAssetSaver interface {
	UpsertGeneratedAsset(ctx context.Context, req *client.UpsertGenAssetReq) (string, error)
}

// Service job 用例编排（pull 模式 + NEED_CONFIRM）。
type Service struct {
	jobRepo     domain.JobRepo
	stepRunRepo domain.StepRunRepo
	tx          dal.TxRunner
	wfRepo      domain.WorkflowLookup
	tplLookup   wfdomain.TemplateLookup
	materials   GeneratedAssetSaver
	jobCfg      *configpb.JobConfig
}

func NewService(jr domain.JobRepo, sr domain.StepRunRepo, tx dal.TxRunner, wfRepo domain.WorkflowLookup, tl wfdomain.TemplateLookup, mc GeneratedAssetSaver, jobCfg *configpb.JobConfig) *Service {
	return &Service{jobRepo: jr, stepRunRepo: sr, tx: tx, wfRepo: wfRepo, tplLookup: tl, materials: mc, jobCfg: jobCfg}
}

// —— SubmitJob ——
type SubmitCommand struct {
	WorkflowID      string
	WorkflowVersion int
	Input           domain.JobInput
	SubmittedBy     string
}

func (s *Service) Submit(ctx context.Context, cmd SubmitCommand) (*domain.Job, error) {
	// 加载 workflow
	wf, err := s.wfRepo.Get(ctx, cmd.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow %s: %w", cmd.WorkflowID, err)
	}
	if wf.Version != cmd.WorkflowVersion {
		return nil, fmt.Errorf("workflow version mismatch: want %d, got %d", wf.Version, cmd.WorkflowVersion)
	}
	// 闸2: bundle vs input_schema
	if err := s.validateBundle(wf.InputSchema, cmd.Input); err != nil {
		return nil, err
	}
	// 创建 job + step_runs
	jobID := "job_" + uuid.NewString()
	job := &domain.Job{
		ID: jobID, Scope: wf.Scope, WorkflowID: wf.ID, WorkflowVersion: wf.Version,
		Input: cmd.Input, Status: domain.JobPending, SubmittedBy: cmd.SubmittedBy,
		SubmittedAt: time.Now(),
	}
	// 为每个 step 创建 step_run，反范式化 template 字段
	stepRuns := make([]*domain.StepRun, 0, len(wf.Steps))
	for _, step := range wf.Steps {
		// 判断 root（无 STEP_OUTPUT 依赖）→ PENDING；否则 BLOCKED
		isRoot := true
		for _, b := range step.Bindings {
			if b.Source == wfdomain.SourceStepOutput {
				isRoot = false
				break
			}
		}
		status := domain.SRBlocked
		if isRoot {
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
			Status: status, ResolvedInputs: map[string]any{}, Candidates: []domain.Candidate{},
			Platform: tpl.Platform, GenWay: tpl.GenWay, GenType: tpl.GenType,
			AssetKind: tpl.OutputAssetKind, NeedsConfirm: tpl.NeedsConfirm, CandidateCount: tpl.CandidateCount,
		}
		stepRuns = append(stepRuns, sr)
	}
	if err := s.jobRepo.Save(ctx, job, stepRuns); err != nil {
		return nil, fmt.Errorf("save job: %w", err)
	}
	// Job → RUNNING
	_ = s.jobRepo.UpdateStatus(ctx, jobID, domain.JobRunning, "")
	return s.jobRepo.Get(ctx, jobID)
}

// validateBundle 闸2: bundle vs input_schema。
func (s *Service) validateBundle(schema []wfdomain.InputSchemaEntry, input domain.JobInput) error {
	matNames := make(map[string]bool)
	for _, m := range input.Materials {
		matNames[m.Name] = true
	}
	assetNames := make(map[string]bool)
	for _, a := range input.Assets {
		assetNames[a.Name] = true
	}
	for _, e := range schema {
		if !e.Required {
			continue
		}
		switch e.ResourceType {
		case "MATERIAL":
			if !matNames[e.Name] {
				return fmt.Errorf("missing required material input: %s", e.Name)
			}
		case "ASSET":
			if !assetNames[e.Name] {
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

// resolveAndRender 解析 binding → resolved slot 值 + 渲染 prompt。
func (s *Service) resolveAndRender(ctx context.Context, sr *domain.StepRun, tpl *wfdomain.TemplateSnapshot, input domain.JobInput) (map[string]any, string, []ClaimAsset) {
	resolved := map[string]any{}
	assets := []ClaimAsset{}

	// 加载 workflow 取 bindings
	job, err := s.jobRepo.Get(ctx, sr.JobID)
	if err != nil {
		return resolved, tpl.PromptBody, assets
	}
	wf, err := s.wfRepo.Get(ctx, job.WorkflowID)
	if err != nil {
		return resolved, tpl.PromptBody, assets
	}
	// 找当前 step 的 bindings
	var bindings []wfdomain.Binding
	for _, step := range wf.Steps {
		if step.Key == sr.StepKey {
			bindings = step.Bindings
			break
		}
	}
	// 加载同 job 的所有 step_run（STEP_OUTPUT 解析用）
	allSRs, _ := s.stepRunRepo.ListByJob(ctx, sr.JobID)
	srByKey := make(map[string]*domain.StepRun, len(allSRs))
	for _, s2 := range allSRs {
		srByKey[s2.StepKey] = s2
	}

	// 解析每个 binding
	prompt := tpl.PromptBody
	for _, b := range bindings {
		var url, content string
		switch b.Source {
		case wfdomain.SourceWorkflowInput:
			// 从 bundle 按 to_input(=slot.name) 取
			url, content = s.resolveFromBundle(b.ToInput, tpl, input)
		case wfdomain.SourceStepOutput:
			// 从前驱 step_run 的选定 candidate 取 url
			if prev, ok := srByKey[b.Ref]; ok && prev.SelectedIndex != nil && *prev.SelectedIndex < len(prev.Candidates) {
				c := prev.Candidates[*prev.SelectedIndex]
				url = c.URL
				content = c.Content
			}
		case wfdomain.SourceConst:
			content = b.ConstValue
		}
		resolved[b.ToInput] = map[string]any{"url": url, "content": content}
		assets = append(assets, ClaimAsset{Label: b.ToInput, URL: url, Content: content})
		// 渲染 prompt：替换 {slot.url} / {slot.content}
		prompt = strings.ReplaceAll(prompt, "{"+b.ToInput+".url}", url)
		prompt = strings.ReplaceAll(prompt, "{"+b.ToInput+".content}", content)
	}
	return resolved, prompt, assets
}

// resolveFromBundle 从 JobInput bundle 按 slot.name 取 url/content。
// slot.name 对 MATERIAL/ASSET = asset_kind；需在 bundle 的 material[]/asset[] 中按 name 找。
func (s *Service) resolveFromBundle(slotName string, tpl *wfdomain.TemplateSnapshot, input domain.JobInput) (string, string) {
	// 查 slot 的 resource_type
	for _, slot := range tpl.Inputs {
		if slot.Name != slotName {
			continue
		}
		switch slot.ResourceType {
		case "MATERIAL":
			for _, m := range input.Materials {
				if m.Name == slotName {
					return m.URL, m.Content
				}
			}
		case "ASSET":
			for _, a := range input.Assets {
				if a.Name == slotName {
					return a.URL, a.Content
				}
			}
		case "PRODUCT":
			if input.Product != nil {
				return "", input.Product.Name // product 无 url，取 name
			}
		case "VIRTUAL_IP":
			if input.VirtualIP != nil {
				return input.VirtualIP.AvatarURL, input.VirtualIP.Name
			}
		case "TEXT":
			// TEXT slot 无 bundle 对应，走 const
		}
	}
	return "", ""
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
	if sr.NeedsConfirm {
		// → NEED_CONFIRM
		_ = s.stepRunRepo.UpdateStatus(ctx, stepRunID, domain.SRNeedConfirm, nil)
		_ = s.jobRepo.UpdateStatus(ctx, sr.JobID, domain.JobNeedConfirm, "")
	} else {
		// 直接 SUCCEEDED
		primary := candidates[0]
		for _, c := range candidates {
			if c.IsPrimary {
				primary = c
				break
			}
		}
		if err := s.finalizeStepRun(ctx, sr, primary); err != nil {
			return nil, err
		}
	}
	return s.stepRunRepo.Get(ctx, stepRunID)
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
	if err := s.finalizeStepRun(ctx, sr, selected); err != nil {
		return nil, err
	}
	_ = s.stepRunRepo.UpdateStatus(ctx, stepRunID, domain.SRSucceeded, map[string]any{"selected_index": selectedIndex})
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
	_ = s.stepRunRepo.UpdateStatus(ctx, stepRunID, domain.SRPending, map[string]any{"candidates": "[]"})
	_ = s.jobRepo.UpdateStatus(ctx, sr.JobID, domain.JobRunning, "")
	return sr, nil
}

// —— FailStepRun ——
func (s *Service) Fail(ctx context.Context, stepRunID string, reason string) (*domain.StepRun, error) {
	sr, err := s.stepRunRepo.Get(ctx, stepRunID)
	if err != nil {
		return nil, err
	}
	_ = s.stepRunRepo.UpdateFailure(ctx, stepRunID, reason)
	_ = s.stepRunRepo.UpdateStatus(ctx, stepRunID, domain.SRFailed, nil)
	_ = s.jobRepo.UpdateStatus(ctx, sr.JobID, domain.JobFailed, reason)
	return sr, nil
}

// —— CancelJob ——
func (s *Service) Cancel(ctx context.Context, jobID string) (*domain.Job, error) {
	_ = s.jobRepo.UpdateStatus(ctx, jobID, domain.JobCancelled, "cancelled by user")
	return s.jobRepo.Get(ctx, jobID)
}

// —— RetryJob ——
func (s *Service) Retry(ctx context.Context, jobID string, reason string, inputOverride *domain.JobInput) (*domain.Job, error) {
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status != domain.JobFailed {
		return nil, fmt.Errorf("job %s status %s, expected failed", jobID, job.Status)
	}
	maxRetries := 3
	if s.jobCfg != nil && s.jobCfg.MaxRetries > 0 {
		maxRetries = int(s.jobCfg.MaxRetries)
	}
	if job.RetryCount >= maxRetries {
		return nil, errors.New("max retries exceeded")
	}
	// 重置 failed/skipped/need_confirm step_runs → pending（仅重置非 succeeded 的）
	// succeeded 的保留；failed → pending；skipped/need_confirm → pending（重新评估依赖）
	stepRuns, err := s.stepRunRepo.ListByJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	// 先把 failed → pending，skipped → blocked（重新等依赖评估）
	for _, sr := range stepRuns {
		switch sr.Status {
		case domain.SRFailed:
			_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRPending, nil)
		case domain.SRSkipped:
			_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRBlocked, nil)
		case domain.SRNeedConfirm:
			_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRPending, map[string]any{"candidates": "[]"})
		}
	}
	// 重新跑 DAG 推进：把能解锁的 blocked → pending
	s.advanceDAG(ctx, jobID, "")
	// input override
	if inputOverride != nil {
		_ = s.jobRepo.UpdateInput(ctx, jobID, *inputOverride)
	}
	// 追加 retry history + count
	record := domain.RetryRecord{RetryAt: time.Now(), Reason: reason, InputOverride: inputOverride}
	_ = s.jobRepo.AppendRetryHistory(ctx, jobID, record, job.RetryCount+1)
	return s.jobRepo.Get(ctx, jobID)
}

// —— GetJob / ListJobs ——
// -- RetryStepRun（重跑单步 + 下游）--
// RetryStepRun 重置 failed/skipped 的 step_run -> pending + retry_count+1，并重置其下游 -> blocked；
// input_override 覆盖 job.input bundle（Claim 时重新解析）。Job -> running，advanceDAG 推进。
func (s *Service) RetryStepRun(ctx context.Context, stepRunID, reason string, inputOverride *domain.JobInput) (*domain.StepRun, error) {
	sr, err := s.stepRunRepo.Get(ctx, stepRunID)
	if err != nil {
		return nil, err
	}
	if sr.Status != domain.SRFailed && sr.Status != domain.SRSkipped {
		return nil, fmt.Errorf("step_run %s status %s, expected failed/skipped", stepRunID, sr.Status)
	}
	updates := map[string]any{
		"retry_count": sr.RetryCount + 1,
		"candidates":  "[]", "failure_reason": "", "claimed_by": "", "output_asset_id": "",
	}
	_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRPending, updates)
	s.resetDownstream(ctx, sr.JobID, sr.StepKey)
	if inputOverride != nil {
		_ = s.jobRepo.UpdateInput(ctx, sr.JobID, *inputOverride)
	}
	job, err := s.jobRepo.Get(ctx, sr.JobID)
	if err == nil {
		_ = s.jobRepo.UpdateStatus(ctx, sr.JobID, domain.JobRunning, "")
		record := domain.RetryRecord{RetryAt: time.Now(), Reason: reason, InputOverride: inputOverride}
		_ = s.jobRepo.AppendRetryHistory(ctx, sr.JobID, record, job.RetryCount+1)
	}
	s.advanceDAG(ctx, sr.JobID, sr.StepKey)
	return s.stepRunRepo.Get(ctx, sr.ID)
}

// resetDownstream 重置依赖 completedStepKey 输出的下游 step_run（非 succeeded）-> blocked。
func (s *Service) resetDownstream(ctx context.Context, jobID, completedStepKey string) {
	job, err := s.jobRepo.Get(ctx, jobID)
	if err != nil {
		return
	}
	wf, err := s.wfRepo.Get(ctx, job.WorkflowID)
	if err != nil {
		return
	}
	downstreamKeys := make(map[string]bool)
	for _, step := range wf.Steps {
		for _, b := range step.Bindings {
			if b.Source == wfdomain.SourceStepOutput && b.Ref == completedStepKey {
				downstreamKeys[step.Key] = true
			}
		}
	}
	if len(downstreamKeys) == 0 {
		return
	}
	allSRs, _ := s.stepRunRepo.ListByJob(ctx, jobID)
	for _, sr := range allSRs {
		if !downstreamKeys[sr.StepKey] {
			continue
		}
		if sr.Status != domain.SRSucceeded {
			_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRBlocked, nil)
		}
	}
}

// GetStepRun 读单条 step_run（只读）。
func (s *Service) GetStepRun(ctx context.Context, stepRunID string) (*domain.StepRun, error) {
	return s.stepRunRepo.Get(ctx, stepRunID)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Job, []*domain.StepRun, error) {
	return s.jobRepo.GetWithStepRuns(ctx, id)
}

func (s *Service) List(ctx context.Context, q domain.ListQuery) ([]*domain.Job, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	return s.jobRepo.List(ctx, q)
}

// —— finalizeStepRun: 落产物 + SUCCEEDED + 推进 DAG ——
func (s *Service) finalizeStepRun(ctx context.Context, sr *domain.StepRun, selected domain.Candidate) error {
	// 1. 调 materials 落 generated_asset
	job, err := s.jobRepo.Get(ctx, sr.JobID)
	if err != nil {
		return err
	}
	assetID, err := s.materials.UpsertGeneratedAsset(ctx, &client.UpsertGenAssetReq{
		Scope: job.Scope, AssetKind: sr.AssetKind,
		Name: safeName(sr.StepKey, sr.ID),
		URL:  selected.URL, Content: selected.Content,
		OwnerUserID: job.SubmittedBy, Visibility: "private",
		JobID: sr.JobID, StepRunID: sr.ID, TemplateID: sr.TemplateID, TemplateVersion: sr.TemplateVersion,
	})
	if err != nil {
		return fmt.Errorf("upsert generated_asset: %w", err)
	}
	// 2. 更新 step_run: output_asset_id + SUCCEEDED
	_ = s.stepRunRepo.UpdateOutput(ctx, sr.ID, assetID)
	_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRSucceeded, nil)
	// 3. 推进 DAG: 下游 BLOCKED → PENDING
	s.advanceDAG(ctx, sr.JobID, sr.StepKey)
	// 4. 检查 sink 是否 SUCCEEDED → Job SUCCEEDED
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
	// step_key → step（含 bindings）
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
	for _, sr := range blocked {
		step, ok := stepByKey[sr.StepKey]
		if !ok {
			continue
		}
		// 收集该 step 的 STEP_OUTPUT 依赖
		allDepsSucceeded := true
		anyDepFailed := false
		for _, b := range step.Bindings {
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
		} else if allDepsSucceeded {
			_ = s.stepRunRepo.UpdateStatus(ctx, sr.ID, domain.SRPending, nil)
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

package client

import (
	"context"
	"fmt"

	jobsvc "media_agent/hertz_gen/media_workflow/job_service"
	job "media_agent/hertz_gen/model/media_workflow/job"
	gendomain "media_agent/media_gen/biz/domain/generation"
)

// WorkflowClient 实现 generation.Repository 端口，包装 hz 生成的 media_workflow.JobServiceClient。
//
// 由 main.go 装配为 generation.Service 的 repo 入参。
// 仅本文件（biz/shared/client/）允许 import hertz_gen/media_workflow/job_service（架构规则：
// 任何文件 import hertz_gen/<app>/** -> 仅 <app>/biz/shared/client/ 允许）。
type WorkflowClient struct {
	cli jobsvc.Client // 由 Init() 装配的全局 MediaWorkflowJobClient
}

// NewWorkflowClient 构造 generation.Repository 的实现。
// cli: 已装配的 media_workflow.JobServiceClient。
func NewWorkflowClient(cli jobsvc.Client) *WorkflowClient {
	return &WorkflowClient{cli: cli}
}

// Compile-time assertion: WorkflowClient 实现 generation.Repository。
var _ gendomain.Repository = (*WorkflowClient)(nil)

// Claim 调 media_workflow.ClaimStepRun 按 (platform, gen_way, gen_type, worker_id) 领取 StepRun。
// 返回 StepRunID=="" 表示无待领（caller 应静默等待下一轮）。
func (c *WorkflowClient) Claim(ctx context.Context, q gendomain.ClaimQuery) (*gendomain.ClaimResult, error) {
	resp, _, err := c.cli.ClaimStepRun(ctx, &job.ClaimStepRunReq{
		Platform: q.Platform,
		GenWay:   q.GenWay,
		GenType:  q.GenType,
		WorkerId: q.WorkerID,
	})
	if err != nil {
		return nil, fmt.Errorf("call media_workflow.ClaimStepRun: %w", err)
	}
	if resp == nil || resp.GetStepRunId() == "" {
		return nil, nil // 无待领
	}
	assets := make([]gendomain.InputAsset, 0, len(resp.GetInputAssets()))
	for _, a := range resp.GetInputAssets() {
		assets = append(assets, gendomain.InputAsset{
			Label:   a.GetLabel(),
			URL:     a.GetUrl(),
			Content: a.GetContent(),
		})
	}
	return &gendomain.ClaimResult{
		StepRunID:      resp.GetStepRunId(),
		JobID:          resp.GetJobId(),
		RenderedPrompt: resp.GetRenderedPrompt(),
		InputAssets:    assets,
		Platform:       resp.GetPlatform(),
		GenWay:         resp.GetGenWay(),
		GenType:        resp.GetGenType(),
		AssetKind:      resp.GetAssetKind(),
		CandidateCount: int(resp.GetCandidateCount()),
	}, nil
}

// Complete 调 media_workflow.CompleteStepRun 回写候选产物。
func (c *WorkflowClient) Complete(ctx context.Context, stepRunID string, candidates []gendomain.Candidate) (*gendomain.CompleteResult, error) {
	pbCands := make([]*job.Candidate, len(candidates))
	for i, cand := range candidates {
		pbCands[i] = &job.Candidate{
			Index:     int32(cand.Index),
			Url:       cand.URL,
			Content:   cand.Content,
			IsPrimary: cand.IsPrimary,
		}
	}
	resp, _, err := c.cli.CompleteStepRun(ctx, &job.CompleteStepRunReq{
		StepRunId:  stepRunID,
		Candidates: pbCands,
	})
	if err != nil {
		return nil, fmt.Errorf("call media_workflow.CompleteStepRun: %w", err)
	}
	status := ""
	if resp != nil {
		status = resp.GetStatus().String()
	}
	return &gendomain.CompleteResult{StepRunID: stepRunID, Status: status}, nil
}

// Fail 调 media_workflow.FailStepRun 回写失败原因。
func (c *WorkflowClient) Fail(ctx context.Context, stepRunID, reason string) (*gendomain.FailResult, error) {
	resp, _, err := c.cli.FailStepRun(ctx, &job.FailStepRunReq{
		StepRunId: stepRunID,
		Reason:    reason,
	})
	if err != nil {
		return nil, fmt.Errorf("call media_workflow.FailStepRun: %w", err)
	}
	status := ""
	if resp != nil {
		status = resp.GetStatus().String()
	}
	return &gendomain.FailResult{StepRunID: stepRunID, Status: status, Reason: reason}, nil
}

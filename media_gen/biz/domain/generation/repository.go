package generation

import "context"

// Repository 是调 media_workflow execution 域的端口（Claim/Complete/Fail 三段协议）。
// 由 biz/shared/client/workflow.go 实现，main.go 注入。
//
// 接口而非直引 hertz_gen client stub：service 不直接 import hertz_gen/<app>（架构规则：
// 任何文件 import hertz_gen/<app>/**（下游 client stub）-> 仅 <app>/biz/shared/client/ 允许）。
type Repository interface {
	// Claim 调 media_workflow.ClaimStepRun 领取 StepRun。返回 StepRunID=="" 表示无待领。
	Claim(ctx context.Context, q ClaimQuery) (*ClaimResult, error)
	// Complete 调 media_workflow.CompleteStepRun 回写候选产物。
	Complete(ctx context.Context, stepRunID string, candidates []Candidate) (*CompleteResult, error)
	// Fail 调 media_workflow.FailStepRun 回写失败原因。
	Fail(ctx context.Context, stepRunID, reason string) (*FailResult, error)
}

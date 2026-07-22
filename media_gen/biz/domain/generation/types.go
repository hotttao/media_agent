// Package generation 定义 media_gen 单次 StepRun 执行的领域模型。
//
// 本域不持久化（无表），仅持有 Claim/Complete/Fail 三段协议的值对象与端口。
// job 状态机归 media_workflow，本域只负责执行一次 StepRun 并回写候选。
package generation

import "time"

// ClaimQuery -- 调 media_workflow.ClaimStepRun 的入参（对齐 ClaimStepRunReq）。
type ClaimQuery struct {
	WorkerID string
	Platform string
	GenWay   string
	GenType  string
}

// InputAsset -- Claim 返回的输入资产。
type InputAsset struct {
	Label   string
	URL     string
	Content string
}

// ClaimResult -- Claim 返回的 StepRun 执行上下文（对齐 ClaimStepRunResp）。
// 若 StepRunID == "" 表示无待领，worker loop 应静默等待下一轮。
type ClaimResult struct {
	StepRunID      string
	JobID          string
	RenderedPrompt string
	InputAssets    []InputAsset
	Platform       string
	GenWay         string
	GenType        string
	AssetKind      string
	CandidateCount int
}

// Candidate -- 产物候选，对齐 media_workflow.job.Candidate。
type Candidate struct {
	Index     int
	URL       string
	Content   string
	IsPrimary bool
}

// CompleteResult -- CompleteStepRun 回写后的 StepRun 状态（对齐 StepRun 摘要）。
type CompleteResult struct {
	StepRunID string
	Status    string // SUCCEEDED / NEED_CONFIRM / FAILED
}

// FailResult -- FailStepRun 回写后的 StepRun 状态。
type FailResult struct {
	StepRunID string
	Status    string // FAILED
	Reason    string
}

// NowFn 可注入的时钟，便于测试。生产用 time.Now。
var NowFn = func() time.Time { return time.Now() }

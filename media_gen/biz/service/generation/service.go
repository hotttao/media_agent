// Package generation 实现单次 StepRun 的执行：Claim -> 生成(mock) -> Complete/Fail。
//
// 不持久化、不调下游生成器（本期 mock）。
// 调 media_workflow RPC 通过 generation.Repository 端口，由 shared/client/workflow.go 实现。
package generation

import (
	"context"
	"fmt"

	gendomain "media_agent/media_gen/biz/domain/generation"
)

// Service 编排单次 StepRun 执行。实现 worker.StepExecutor 端口。
type Service struct {
	repo gendomain.Repository
}

// NewService 装配 generation service。
// repo: media_workflow execution 域的客户端（Claim/Complete/Fail）。
func NewService(repo gendomain.Repository) *Service {
	return &Service{repo: repo}
}

// ExecuteOnce 执行单次 Claim -> Generate(mock) -> Complete/Fail。
// 实现 worker.StepExecutor 接口。
//
// 流程：
//  1. Claim 按 (workerID, platform, gen_way, gen_type) 领取 StepRun
//  2. 若返回 StepRunID=="" 表示无待领，直接 return nil
//  3. GenerateCandidates(mock): 按 candidate_count 产 N 个假候选
//  4. Complete 回写候选；失败则 Fail 回写原因
func (s *Service) ExecuteOnce(ctx context.Context, workerID, platform, genWay, genType string) error {
	// TODO(real-impl): 从 worker registry 取 (platform, gen_way, gen_type)。
	// 当前签名只收 workerID，platform/gen_way/gen_type 由调用方在 loop 内传入或由 registry 查询。
	// 本期 mock loop 不实际跑，签名保持最简。
	// 真实实现时 ExecuteOnce 签名可能扩展为 (ctx, workerInfo) 或查询 registry。
	return s.executeOnceWith(ctx, workerID, platform, genWay, genType)
}

// executeOnceWith 内部入口，可指定 platform/gen_way/gen_type（便于真实实现扩展签名）。
func (s *Service) executeOnceWith(ctx context.Context, workerID, platform, genWay, genType string) error {
	claim, err := s.repo.Claim(ctx, gendomain.ClaimQuery{
		WorkerID: workerID, Platform: platform, GenWay: genWay, GenType: genType,
	})
	if err != nil {
		return fmt.Errorf("claim step run: %w", err)
	}
	if claim == nil || claim.StepRunID == "" {
		// 无待领 StepRun，静默返回。
		return nil
	}

	candidates := s.GenerateCandidates(claim.StepRunID, claim.RenderedPrompt, claim.CandidateCount)

	if _, err := s.repo.Complete(ctx, claim.StepRunID, candidates); err != nil {
		// Complete 失败：尝试 Fail 回写（best-effort），不本地 retry
		// （media_workflow scanner 会回收 claimed 超时的 step_run 重新入队）。
		if _, ferr := s.repo.Fail(ctx, claim.StepRunID, fmt.Sprintf("complete failed: %v", err)); ferr != nil {
			return fmt.Errorf("complete failed: %v; fail also failed: %w", err, ferr)
		}
		return fmt.Errorf("complete failed (fail written): %w", err)
	}
	return nil
}

// GenerateCandidates 本地生成(mock) 候选产物。
//
// 规则：
//   - count <= 0 -> 默认 1
//   - count == 1 -> 单候选 is_primary=true（主候选直接作为产物）
//   - count > 1  -> N 候选全 is_primary=false（交用户选）
//
// 本期 mock：不消费 renderedPrompt，直接构造假 url/content。
// 真实实现预期调下游生成器（GPT / 即梦 / 其他）。
func (s *Service) GenerateCandidates(stepRunID, renderedPrompt string, count int) []gendomain.Candidate {
	if count <= 0 {
		count = 1
	}
	out := make([]gendomain.Candidate, count)
	isPrimary := count == 1
	for i := 0; i < count; i++ {
		out[i] = gendomain.Candidate{
			Index:     i,
			URL:       fmt.Sprintf("mock://generated/%s/%d", stepRunID, i),
			Content:   fmt.Sprintf("mock content for %s#%d", stepRunID, i),
			IsPrimary: isPrimary,
		}
	}
	return out
}

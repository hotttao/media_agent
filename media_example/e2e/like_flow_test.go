//go:build e2e

// like_flow_test.go：点赞流程（跨 domain 事务）e2e。
//
// 验证 workflow/like_flow 的同事务一致性：
//   - 提交：like + counter 都生效；
//   - 回滚：counter 失败时 like 也被撤销（事务整体回滚）。
package e2e

import (
	"context"
	"testing"

	"media_agent/media_example/biz/workflow"
	"media_agent/media_example/e2e/fixtures"
)

// TestLikeFlow_Commit：正常点赞 → likes 有记录 + post_counters.likes_count=1。
func TestLikeFlow_Commit(t *testing.T) {
	env := fixtures.NewLikeFlowEnv(t)
	ctx := context.Background()

	if err := env.Flow.Like(ctx, workflow.LikeCommand{UserID: "u1", PostID: "p1"}); err != nil {
		t.Fatalf("Like: %v", err)
	}
	if got := fixtures.LikesCount(t, env.DB, "p1"); got != 1 {
		t.Errorf("likes_count: want 1, got %d", got)
	}
	if !fixtures.LikeExists(t, env.DB, "u1", "p1") {
		t.Errorf("likes row (u1,p1) should exist after commit")
	}
}

// TestLikeFlow_Rollback：counter 失败 → 整体回滚，likes 无残留。
func TestLikeFlow_Rollback(t *testing.T) {
	env := fixtures.NewLikeFlowEnvWithFailingCounter(t)

	err := env.Flow.Like(context.Background(), workflow.LikeCommand{UserID: "u2", PostID: "p2"})
	if err == nil {
		t.Fatal("Like should fail when counter fails")
	}
	if fixtures.LikeExists(t, env.DB, "u2", "p2") {
		t.Errorf("likes row (u2,p2) should NOT exist after rollback")
	}
	if got := fixtures.LikesCount(t, env.DB, "p2"); got != 0 {
		t.Errorf("likes_count after rollback: want 0, got %d", got)
	}
}

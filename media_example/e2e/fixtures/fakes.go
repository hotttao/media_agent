// fakes.go：测试替身（fake），用于注入失败等场景。
//
// 放 fixtures 包供多个 e2e 测试复用；不进生产代码。
package fixtures

import (
	"context"
	"fmt"

	domaincounter "media_agent/media_example/biz/domain/counter"
)

// FailingCounterRepo 实现 domaincounter.Repo，IncrLikes 恒失败。
// 用于 LikeFlow 回滚测试：counter 失败 → 事务回滚 → likes 插入被撤销。
type FailingCounterRepo struct{}

// IncrLikes 恒返回错误。
func (FailingCounterRepo) IncrLikes(ctx context.Context, postID string) error {
	return fmt.Errorf("injected counter failure")
}

// 编译期断言：FailingCounterRepo 实现 domaincounter.Repo。
var _ domaincounter.Repo = FailingCounterRepo{}

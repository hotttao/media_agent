//go:build e2e

// app.go：应用层 fixture——把真实 repo/service/workflow 装配好供测试用。
//
// 测试只声明「我要一个跑在隔离 database 上的 LikeFlow」，不管装配细节。
// 未来新增 service/workflow 时在此扩展（如 NewOrderFlowEnv）。
package fixtures

import (
	"testing"

	"gorm.io/gorm"

	"media_agent/media_example/biz/dal"
	dalcounter "media_agent/media_example/biz/dal/repo/counter"
	dallike "media_agent/media_example/biz/dal/repo/like"
	"media_agent/media_example/biz/service/counter"
	"media_agent/media_example/biz/service/like"
	"media_agent/media_example/biz/workflow"
)

// LikeFlowEnv 是点赞流程的测试环境：含真实 db + 装配好的 LikeFlow。
type LikeFlowEnv struct {
	DB         *gorm.DB
	Flow       *workflow.LikeFlow
	LikeSvc    *like.Service
	CounterSvc *counter.Service
}

// NewLikeFlowEnv 装配一个跑在隔离 database 上的 LikeFlow（真实 repo + service + workflow）。
// 内部调 NewDatabase + Migrate，测试结束自动清理（database 删除 + 连接关闭）。
func NewLikeFlowEnv(t *testing.T) *LikeFlowEnv {
	t.Helper()
	db := NewDatabase(t)
	Migrate(t, db)

	tx := dal.NewTxRunner(db.Gorm)
	likeSvc := like.NewService(dallike.NewRepo(db.Gorm))
	counterSvc := counter.NewService(dalcounter.NewRepo(db.Gorm))
	return &LikeFlowEnv{
		DB:         db.Gorm,
		LikeSvc:    likeSvc,
		CounterSvc: counterSvc,
		Flow:       workflow.NewLikeFlow(tx, likeSvc, counterSvc),
	}
}

// NewLikeFlowEnvWithFailingCounter 同 NewLikeFlowEnv，但 counter 用 FailingCounterRepo，
// 用于回滚测试（counter 失败 → 事务回滚）。
func NewLikeFlowEnvWithFailingCounter(t *testing.T) *LikeFlowEnv {
	t.Helper()
	db := NewDatabase(t)
	Migrate(t, db)

	tx := dal.NewTxRunner(db.Gorm)
	likeSvc := like.NewService(dallike.NewRepo(db.Gorm))
	counterSvc := counter.NewService(FailingCounterRepo{}) // counter 必失败
	return &LikeFlowEnv{
		DB:         db.Gorm,
		LikeSvc:    likeSvc,
		CounterSvc: counterSvc,
		Flow:       workflow.NewLikeFlow(tx, likeSvc, counterSvc),
	}
}

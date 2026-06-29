//go:build e2e

// Package e2e 存放 media_example 的端到端测试（go test + testcontainers）。
//
// 组织约定（harness/contributing/e2e.md「e2e 测试组织」）：
//   - 本文件（e2e_test.go）：build tag 声明 + TestMain（进程级共享资源 MySQL 容器）；
//   - <topic>_test.go：按主题分文件（migration_test.go / like_flow_test.go / ...）；
//   - fixtures/：资源 fixture（NewXxx(t) + t.Cleanup teardown）+ 测试替身。
//
// 新增 e2e 测试：在 e2e/ 下按主题新建 <topic>_test.go，用 fixtures 组合资源，写断言。
// 新增资源：在 fixtures/ 加 <resource>.go，写 New<Res>(t) + t.Cleanup。
//
// 运行：go test -tags=e2e ./e2e/ -v（需 docker；无 docker 自动 skip）。
package e2e

import (
	"context"
	"log"
	"os"
	"testing"

	"media_agent/media_example/e2e/fixtures"
)

// TestMain 启动进程级共享 MySQL 容器，供所有 e2e 测试复用（各测试用独立 database 隔离）。
//
// 共享策略：容器只起一次（省启动开销）；各测试经 fixtures.NewDatabase 派生独立 database，
// 互不影响，测试结束自动删 database。无 docker 时 skip 整个 e2e 包。
func TestMain(m *testing.M) {
	ctx := context.Background()
	mysql, err := fixtures.StartSharedMySQL(ctx)
	if err != nil {
		log.Printf("e2e: start shared mysql (need docker), skip: %v", err)
		os.Exit(0) // 无 docker 不算失败
	}
	defer func() {
		if err := mysql.Terminate(ctx); err != nil {
			log.Printf("e2e: terminate mysql: %v", err)
		}
	}()
	fixtures.SetSharedMySQL(mysql) // 注入到 fixtures 供 NewDatabase 使用
	os.Exit(m.Run())
}

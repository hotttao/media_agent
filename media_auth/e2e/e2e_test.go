//go:build e2e

// Package e2e 存放 media_auth 的端到端测试（go test + testcontainers pg）。
//
// 组织（harness/contributing/e2e.md）：e2e_test.go = build tag + TestMain（共享 pg 容器）；
// <domain>_test.go = 按主题；fixtures/ = 资源 fixture + 装配。
//
// 运行：go test -tags=e2e ./e2e/ -v（需 docker；无 docker 自动 skip）。
package e2e

import (
	"context"
	"log"
	"os"
	"testing"

	"media_agent/media_auth/e2e/fixtures"
)

// TestMain 启动进程级共享 pg 容器，供所有 e2e 测试复用（各测试独立 database 隔离）。
func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, err := fixtures.StartSharedPG(ctx)
	if err != nil {
		log.Printf("e2e: start shared pg (need docker), skip: %v", err)
		os.Exit(0)
	}
	defer func() { _ = pg.Terminate(ctx) }()
	fixtures.SetSharedPG(pg)
	os.Exit(m.Run())
}

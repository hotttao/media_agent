// Package e2e 是 media-cli 的 e2e 资源编排框架。
//
// 核心抽象：Resource 接口（tearup/teardown 对等）+ Env 编排器（按序 Up / LIFO Down）。
// 资源模块（resources/）实现 Resource：ComposeStack（被测服务 docker compose）等。
//
// 设计：基础环境（pg/consul/otel）手动起一次常驻，e2e 只管被测服务。
//
// 编排入口 NewEnv(services) 清晰展示资源顺序：
//
//	env := NewEnv("media_auth")     // 只起 media_auth（基础环境已常驻）
//	env.Up(ctx)                     // tearup：build + 起被测服务 + 健康探测
//	// ... 执行测试 ...
//	env.Down(ctx)                   // teardown：停被测服务（不碰基础环境）
package e2e

import (
	"context"
	"fmt"
	"path/filepath"

	"media_agent/media_cli/cmd/e2e/resources"
)

// Resource 是资源模块接口：tearup（Up）与 teardown（Down）对等出现。
type Resource interface {
	Name() string
	Up(ctx context.Context) error
	Down(ctx context.Context) error
}

// Env 是资源编排器：按序 Up，LIFO 逆序 Down。
type Env struct {
	resources []Resource
}

// serviceComposeMap 被测服务名 → compose 文件相对路径（相对 media-cli 工作目录）。
// 被测服务 compose 不依赖基础环境（连 host.docker.internal:5432），可独立 Up。
var serviceComposeMap = map[string]string{
	"media_auth":    "../media_auth/docker-compose.yml",
	"media_example": "../media_example/docker-compose.yml",
}

// NewEnv 创建编排器。services 指定起哪些被测服务（基础环境需已手动起好）。
// 空列表 = 起全部被测服务。
func NewEnv(services []string) *Env {
	if len(services) == 0 {
		for s := range serviceComposeMap {
			services = append(services, s)
		}
	}
	var composeFiles []string
	for _, s := range services {
		if f, ok := serviceComposeMap[s]; ok {
			if abs, err := filepath.Abs(f); err == nil {
				composeFiles = append(composeFiles, abs)
			}
		}
	}
	return &Env{
		resources: []Resource{
			resources.NewComposeStack(composeFiles, services),
		},
	}
}

// Up 按序 tearup 所有资源。
func (e *Env) Up(ctx context.Context) error {
	for _, r := range e.resources {
		fmt.Printf(">> tearup: %s\n", r.Name())
		if err := r.Up(ctx); err != nil {
			return fmt.Errorf("tearup %s: %w", r.Name(), err)
		}
		fmt.Printf("   ✅ %s ready\n", r.Name())
	}
	return nil
}

// Down LIFO 逆序 teardown。
func (e *Env) Down(ctx context.Context) error {
	var firstErr error
	for i := len(e.resources) - 1; i >= 0; i-- {
		r := e.resources[i]
		fmt.Printf(">> teardown: %s\n", r.Name())
		if err := r.Down(ctx); err != nil {
			fmt.Printf("   ⚠️ %s teardown error: %v\n", r.Name(), err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Restart 重启指定资源（Down + Up）。
func (e *Env) Restart(ctx context.Context, name string) error {
	for _, r := range e.resources {
		if r.Name() == name {
			fmt.Printf(">> restart: %s (down)\n", name)
			if err := r.Down(ctx); err != nil {
				return fmt.Errorf("restart down %s: %w", name, err)
			}
			fmt.Printf(">> restart: %s (up)\n", name)
			if err := r.Up(ctx); err != nil {
				return fmt.Errorf("restart up %s: %w", name, err)
			}
			fmt.Printf("   ✅ %s restarted\n", name)
			return nil
		}
	}
	return fmt.Errorf("resource %q not found", name)
}

// Package resources 提供 e2e 资源模块实现。
//
// ComposeStack：用 testcontainers-go compose 模块编排被测服务的 docker-compose 栈。
// 基础环境（pg/consul/otel 等）不由此管理——手动起一次常驻，e2e 只管被测服务。
// Up = compose.Up + 健康探测；Down = compose.Down（只停被测服务，不碰基础环境）。
package resources

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/compose"
)

// ComposeStack 管理被测服务的 docker-compose 栈（含基础 compose 用于 depends_on 解析）。
type ComposeStack struct {
	stack        *compose.DockerCompose
	composeFiles []string // 基础 + 被测服务 compose 文件
	services     []string // 只起这些被测服务（不含基础环境的 postgres 等）
}

// NewComposeStack 创建 compose 栈资源。
// composeFiles：基础 compose + 被测服务 compose（用于解析 depends_on）。
// services：只起这些被测服务（基础环境已常驻，不重复起）。
func NewComposeStack(composeFiles []string, services []string) *ComposeStack {
	return &ComposeStack{composeFiles: composeFiles, services: services}
}

// Name 返回资源名。
func (c *ComposeStack) Name() string { return "compose-stack" }

// Up 启动被测服务 + 健康探测（tearup）。
// 前提：基础环境已手动起好（postgres 常驻）。
func (c *ComposeStack) Up(ctx context.Context) error {
	stack, err := compose.NewDockerCompose(c.composeFiles...)
	if err != nil {
		return fmt.Errorf("create compose: %w", err)
	}
	c.stack = stack

	// 只起被测服务（RunServices），基础环境不重复起。
	// RemoveOrphans=true：清理不属于本次 RunServices 的旧容器。
	upOpts := []compose.StackUpOption{
		compose.Wait(true),
		compose.RemoveOrphans(false),
	}
	if len(c.services) > 0 {
		upOpts = append(upOpts, compose.RunServices(c.services...))
	}
	if err := c.stack.Up(ctx, upOpts...); err != nil {
		return fmt.Errorf("compose up: %w", err)
	}

	// 健康探测：只测实际启动的 services（RunServices 指定）。
	for _, svc := range c.services {
		var url string
		switch svc {
		case "media_auth":
			url = "http://localhost:8001/health"
		case "media_example":
			url = "http://localhost:8000/health"
		default:
			continue
		}
		if err := probeHealth(ctx, url, svc); err != nil {
			return err
		}
	}
	return nil
}

// Down 停止被测服务 + 清理（teardown）。不碰基础环境。
func (c *ComposeStack) Down(ctx context.Context) error {
	if c.stack == nil {
		return nil
	}
	// 只停被测服务（不 RemoveVolumes/RemoveImages，避免误删基础环境数据）。
	return c.stack.Down(ctx, compose.RemoveOrphans(false))
}

// probeHealth 轮询 health 端点直到 200 或超时。
func probeHealth(ctx context.Context, url, name string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 60; i++ {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("%s health probe timeout: %s", name, url)
}

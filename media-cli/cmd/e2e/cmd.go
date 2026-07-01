// cmd.go — media-cli e2e 子命令（up/down/restart/status/run）。
//
// 基础环境（pg/consul/otel）手动起一次常驻：
//
//	docker compose -f docker-compose/docker-compose.yml up -d
//
// e2e 命令只管被测服务（build + 起/停/重启 + 健康探测）。
// 流程：media-cli e2e up [--services media_auth] → 测试 → media-cli e2e down
package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// E2ECmd 是一级子命令 e2e。
var E2ECmd = &cobra.Command{
	Use:   "e2e",
	Short: "e2e 环境编排（tearup/teardown 被测服务）",
}

func init() {
	E2ECmd.AddCommand(upCmd(), downCmd(), restartCmd(), statusCmd(), runCmd())
	cmd.RegisterCmd(E2ECmd)
}

// upCmd media-cli e2e up [--services media_auth] —— tearup 被测服务。
// 前提：基础环境已手动起好。
func upCmd() *cobra.Command {
	var services string
	c := &cobra.Command{
		Use:   "up",
		Short: "tearup：build + 启动被测服务 + 健康探测（基础环境需已常驻）",
		RunE: func(c *cobra.Command, args []string) error {
			env := NewEnv(parseServices(services))
			if err := env.Up(context.Background()); err != nil {
				return err
			}
			fmt.Println("✅ e2e environment ready")
			return nil
		},
	}
	c.Flags().StringVar(&services, "services", "", "被测服务（逗号分隔，如 media_auth；空=全部）")
	return c
}

// downCmd media-cli e2e down —— teardown 被测服务（不碰基础环境）。
func downCmd() *cobra.Command {
	var services string
	c := &cobra.Command{
		Use:   "down",
		Short: "teardown：停止被测服务 + 清理（不碰基础环境）",
		RunE: func(c *cobra.Command, args []string) error {
			env := NewEnv(parseServices(services))
			if err := env.Down(context.Background()); err != nil {
				return err
			}
			fmt.Println("✅ e2e services cleaned")
			return nil
		},
	}
	c.Flags().StringVar(&services, "services", "", "被测服务（逗号分隔；空=全部）")
	return c
}

// restartCmd media-cli e2e restart <resource> —— 单服务重启。
func restartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <resource>",
		Short: "重启单个资源（如 restart compose-stack）",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			env := NewEnv(nil)
			return env.Restart(context.Background(), args[0])
		},
	}
}

// statusCmd media-cli e2e status —— 探测各被测服务健康状态。
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "探测各被测服务健康状态",
		RunE: func(c *cobra.Command, args []string) error {
			services := []struct {
				name string
				url  string
			}{
				{"media_auth", "http://localhost:8001/health"},
				{"media_example", "http://localhost:8000/health"},
			}
			client := &http.Client{Timeout: 2 * time.Second}
			allOK := true
			for _, svc := range services {
				resp, err := client.Get(svc.url)
				if err == nil && resp.StatusCode == 200 {
					resp.Body.Close()
					fmt.Printf("  ✅ %s healthy\n", svc.name)
				} else {
					if resp != nil {
						resp.Body.Close()
					}
					fmt.Printf("  ❌ %s unhealthy (%v)\n", svc.name, err)
					allOK = false
				}
			}
			if !allOK {
				return fmt.Errorf("some services unhealthy")
			}
			return nil
		},
	}
}

// parseServices 解析 --services flag（逗号分隔）。
func parseServices(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

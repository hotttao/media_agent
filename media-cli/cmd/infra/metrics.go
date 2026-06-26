package infra

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// metricsURL Prometheus 独立监听地址（monitor-prometheus server tracer 自管理）。
var metricsURL string

// infraMetricsCmd 测试 /metrics 端点。
var infraMetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "测试 /metrics 端点（:9091）",
	RunE: func(c *cobra.Command, args []string) error {
		status, _, body, err := cmd.DoRequest("GET", metricsURL, nil, nil)
		if err != nil {
			return cmd.Skip("metrics", "connect failed: "+err.Error()+" (prometheus.enabled=false?)")
		}
		if status == 404 {
			return cmd.Skip("metrics", "404 (metrics tracer not mounted; prometheus.enabled=false?)")
		}
		if status != 200 {
			return cmd.Fail("metrics", fmt.Sprintf("status=%d", status))
		}
		if !strings.Contains(body, "# HELP") && !strings.Contains(body, "# TYPE") {
			return cmd.Fail("metrics", "body not prometheus format")
		}
		return cmd.Pass("metrics", fmt.Sprintf("%d bytes", len(body)))
	},
}

func init() {
	infraMetricsCmd.Flags().StringVar(&metricsURL, "metrics-url", "http://127.0.0.1:9091/metrics", "metrics 端点地址（monitor-prometheus 独立监听）")
	infraCmd.AddCommand(infraMetricsCmd)
}

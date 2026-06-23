// infra_metrics.go：GET /metrics 期望 Prometheus 文本格式。
//
// metrics 由 monitor-prometheus server tracer 自管理暴露在独立监听地址
// PrometheusConfig.address（默认 :9091），不在主服务端口。
// --metrics-url 默认 http://127.0.0.1:9091/metrics；prometheus.enabled=false 时
// 连接会失败或返回非 200，本命令据此判定 skip / fail。
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var metricsURL string

var infraMetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "测试 /metrics 端点（:9091）",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, _, body, err := doRequest("GET", metricsURL, nil, nil)
		if err != nil {
			return skip("metrics", "connect failed: "+err.Error()+" (prometheus.enabled=false?)")
		}
		if status == 404 {
			return skip("metrics", "404 (metrics tracer not mounted; prometheus.enabled=false?)")
		}
		if status != 200 {
			return fail("metrics", fmt.Sprintf("status=%d", status))
		}
		if !strings.Contains(body, "# HELP") && !strings.Contains(body, "# TYPE") {
			return fail("metrics", "body not prometheus format")
		}
		return pass("metrics", fmt.Sprintf("%d bytes", len(body)))
	},
}

func init() {
	infraMetricsCmd.Flags().StringVar(&metricsURL, "metrics-url", "http://127.0.0.1:9091/metrics", "metrics 端点地址（monitor-prometheus 独立监听）")
	infraCmd.AddCommand(infraMetricsCmd)
}

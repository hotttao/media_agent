// infra_metrics.go：GET /metrics 期望 Prometheus 文本格式。
//
// 需要 server 配置 prometheus.enabled=true 且 metrics tracer 已挂载。
// 当前 serverhertz metrics 是 no-op 占位，本命令会在 404 时返回 skip。
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var infraMetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "测试 /metrics 端点",
	RunE: func(cmd *cobra.Command, args []string) error {
		// prometheus.address 是独立监听地址，默认 :9091；
		// 这里通过 --host 也可指定，但更稳的是从 server 推断。
		url := strings.TrimRight(FlagHost, "/") + "/metrics"
		status, _, body, err := doRequest("GET", url, nil, nil)
		if err != nil {
			return fail("metrics", err.Error())
		}
		if status == 404 {
			return pass("metrics", "404 (metrics tracer not mounted; prometheus.enabled=false?)")
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
	infraCmd.AddCommand(infraMetricsCmd)
}

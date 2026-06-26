package infra

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

var (
	traceJaegerURL string
	traceService   string
)

// infraTraceCmd 发请求并查 Jaeger 断言 span 入库。
var infraTraceCmd = &cobra.Command{
	Use:   "trace",
	Short: "发请求并查 Jaeger 断言 span 入库",
	RunE: func(c *cobra.Command, args []string) error {
		// 1. 触发一次请求（被 hertztracing 生成 span）。
		url := cmd.FlagHost + "/_test/echo?name=trace-test"
		status, _, _, err := cmd.DoRequest("GET", url, nil, nil)
		if err != nil {
			return cmd.Fail("trace", "trigger request: "+err.Error())
		}
		if status != 200 {
			return cmd.Fail("trace", fmt.Sprintf("trigger status=%d", status))
		}

		// 2. 轮询 Jaeger API 查该 service 的 trace（导出有延迟，重试几秒）。
		apiURL := fmt.Sprintf("%s/api/traces?service=%s&limit=1", traceJaegerURL, traceService)
		var found int
		for i := 0; i < 6; i++ {
			if i > 0 {
				time.Sleep(time.Second)
			}
			n, err := jaegerTraceCount(apiURL)
			if err != nil {
				// Jaeger 不可达（未启动栈）→ skip。
				return cmd.Skip("trace", "jaeger unreachable: "+err.Error())
			}
			if n > 0 {
				found = n
				break
			}
		}
		if found == 0 {
			return cmd.Fail("trace", "no span found in jaeger (otel.enabled=false or collector down?)")
		}
		return cmd.Pass("trace", fmt.Sprintf("%d span(s) in jaeger for service=%s", found, traceService))
	},
}

// jaegerTraceCount 查 Jaeger /api/traces，返回 trace 条数。
func jaegerTraceCount(apiURL string) (int, error) {
	resp, err := cmd.HTTPClient().Get(apiURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("jaeger status=%d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	var v struct {
		Data []struct {
			Spans []any `json:"spans"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return 0, err
	}
	return len(v.Data), nil
}

func init() {
	infraTraceCmd.Flags().StringVar(&traceJaegerURL, "jaeger-url", "http://127.0.0.1:16686", "Jaeger UI 地址")
	infraTraceCmd.Flags().StringVar(&traceService, "service", "media-example", "被测服务名（app.name）")
	infraCmd.AddCommand(infraTraceCmd)
}

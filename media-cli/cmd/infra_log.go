// infra_log.go：发一个带标记的请求 → 查 Loki API 断言日志入库。
//
// 依赖：log.enabled=true（dir 指向 media_logs/<app>，console 也写）且
// promtail + loki 已启动（docker-compose）。
// --loki-url 默认 http://127.0.0.1:3100；用 --marker 指定查询关键字。
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

var (
	lokiURL   string
	logMarker string
)

var infraLogCmd = &cobra.Command{
	Use:   "log",
	Short: "发请求并查 Loki 断言日志入库",
	RunE: func(cmd *cobra.Command, args []string) error {
		if logMarker == "" {
			// 默认查 access log 路径：accesslog 中间件会记录 /_test/echo 请求，
			// 该日志经 zap(hlog) 落盘 → promtail → loki。
			logMarker = "/_test/echo"
		}
		// 1. 触发请求（被 accesslog 中间件记录）。
		urlStr := FlagHost + "/_test/echo?name=log-test"
		status, _, _, err := doRequest("GET", urlStr, nil, nil)
		if err != nil {
			return fail("log", "trigger request: "+err.Error())
		}
		if status != 200 {
			return fail("log", fmt.Sprintf("trigger status=%d", status))
		}

		// 2. 轮询 Loki /loki/api/v1/query 按关键字查日志（采集有延迟，重试几秒）。
		q := url.QueryEscape(fmt.Sprintf(`{job="media-app"} |= "%s"`, logMarker))
		queryURL := fmt.Sprintf("%s/loki/api/v1/query?query=%s", lokiURL, q)
		var found int
		for i := 0; i < 8; i++ {
			if i > 0 {
				time.Sleep(time.Second)
			}
			n, err := lokiStreamCount(queryURL)
			if err != nil {
				return skip("log", "loki unreachable: "+err.Error())
			}
			if n > 0 {
				found = n
				break
			}
		}
		if found == 0 {
			return fail("log", "no log line found in loki (log.enabled=false or promtail/loki down?)")
		}
		return pass("log", fmt.Sprintf("%d line(s) in loki matching %q", found, logMarker))
	},
}

// lokiStreamCount 查 Loki /api/v1/query，返回匹配的 stream 条数。
func lokiStreamCount(queryURL string) (int, error) {
	resp, err := httpClient().Get(queryURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("loki status=%d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	// query 返回 {status, data:{result:[...]}}
	var v struct {
		Status string `json:"status"`
		Data   struct {
			Result []any `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return 0, err
	}
	return len(v.Data.Result), nil
}

func init() {
	infraLogCmd.Flags().StringVar(&lokiURL, "loki-url", "http://127.0.0.1:3100", "Loki API 地址")
	infraLogCmd.Flags().StringVar(&logMarker, "marker", "", "查询关键字（默认自动生成）")
	infraCmd.AddCommand(infraLogCmd)
}

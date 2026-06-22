// infra.go：infra e2e 测试子命令的公共辅助 + 父命令。
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var infraCmd = &cobra.Command{
	Use:   "infra",
	Short: "服务端治理能力 e2e 测试",
}

func init() {
	RegisterCmd(infraCmd)
}

// httpClient 统一 client，带 30s 超时。
func httpClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// doRequest 发请求并返回 status / body / response header。
func doRequest(method, url string, headers map[string]string, body io.Reader) (status int, respHeaders http.Header, respBody string, err error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return 0, nil, "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b), nil
}

// pass 失败时返回 error，让 cobra 退出码非 0；成功打印 ✅。
func pass(label string, detail string) error {
	if FlagVerbose {
		fmt.Printf("✅ %s: %s\n", label, detail)
	} else {
		fmt.Printf("✅ %s\n", label)
	}
	return nil
}

// fail 返回 error，打印 ❌。
func fail(label string, detail string) error {
	return fmt.Errorf("❌ %s: %s", label, detail)
}

// prettyJSON 把 JSON 字符串缩进打印。
func prettyJSON(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// trim 每行去除前后空白，便于断言。
func trim(s string) string {
	return strings.TrimSpace(s)
}

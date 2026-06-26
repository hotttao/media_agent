// testharness.go — media-cli 各 e2e 子命令复用的 HTTP/断言工具。
//
// 历史说明：本文件原本计划在 cmd/internal/testharness/。内联回 cmd 根后，
// subpackage（infra/、client/）直接 import cmd 即可调用 DoRequest / Pass / Fail / Skip。

package cmd

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient 统一 client，带 30s 超时。
func HTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// DoRequest 发请求并返回 status / body / response header。
func DoRequest(method, url string, headers map[string]string, body io.Reader) (status int, respHeaders http.Header, respBody string, err error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return 0, nil, "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := HTTPClient().Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b), nil
}

// Pass 成功时打印 ✅，返回 nil 让 cobra 退出码为 0。
func Pass(label string, detail string) error {
	if FlagVerbose {
		fmt.Printf("✅ %s: %s\n", label, detail)
	} else {
		fmt.Printf("✅ %s\n", label)
	}
	return nil
}

// Fail 返回 error，打印 ❌，让 cobra 退出码非 0。
func Fail(label string, detail string) error {
	return fmt.Errorf("❌ %s: %s", label, detail)
}

// Skip 打印 ⚠️ 并返回 nil（能力未启用时跳过，不算失败）。
func Skip(label string, detail string) error {
	if FlagVerbose {
		fmt.Printf("⚠️  %s: %s\n", label, detail)
	} else {
		fmt.Printf("⚠️  %s\n", label)
	}
	return nil
}

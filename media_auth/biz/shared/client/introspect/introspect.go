// Package introspect 是 OAuth2 token-exchange (RFC 8693) 中调用资源服务器 /introspect
// 端点（RFC 7662）校验 subject_token 的 HTTP client。
//
// 装配在 biz/shared/client/（消费 hertz_infra/clienthertz 治理能力）；service/domain 不直接
// import SDK。对齐 control-plane services/oauth-provider.ts verifySubjectToken。
//
// 信任模型：当前 introspect 调用不带 app 级鉴权，依赖 subject_token 不可伪造 + 内网边界
// （control-plane 现状）。后续可注册 cp 为各 RS 的 confidential client 加 Basic auth。
package introspect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client 调用资源服务器的 /introspect 端点。
type Client struct {
	httpClient *http.Client
}

// NewClient 构造 introspect client。httpClient 为 nil 时用默认 5s 超时 client。
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{httpClient: httpClient}
}

// IntrospectionResponse 是 RFC 7662 introspection 响应（仅取 media_auth 关心的字段）。
type IntrospectionResponse struct {
	Active   bool   `json:"active"`
	Sub      string `json:"sub"`
	ClientID string `json:"client_id"`
}

// VerifySubjectToken 调用 {resourceOrigin}/introspect 校验 subject_token。
//
// 返回 (userID, clientID, true, nil) 当 active 且 sub 非空；否则 (..., false, nil)。
// resourceOrigin 形如 https://rs.example.com（无路径）；本方法拼 /introspect。
// HTTP 错误 / 解析失败 / 超时 → (..., false, err)，由调用方决定如何处理（control-plane 现状返 nil）。
func (c *Client) VerifySubjectToken(ctx context.Context, subjectToken, resourceOrigin string) (userID, clientID string, ok bool, err error) {
	introspectURL := strings.TrimRight(resourceOrigin, "/") + "/introspect"
	form := url.Values{"token": {subjectToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, introspectURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", false, fmt.Errorf("introspect: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", false, fmt.Errorf("introspect: call %s: %w", introspectURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", "", false, fmt.Errorf("introspect: %s -> %d: %s", introspectURL, resp.StatusCode, string(body))
	}

	var data IntrospectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", false, fmt.Errorf("introspect: decode: %w", err)
	}
	if !data.Active || data.Sub == "" {
		return "", "", false, nil
	}
	return data.Sub, data.ClientID, true, nil
}

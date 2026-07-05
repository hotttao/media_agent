// Package mcp 是第三方 MCP server OAuth 端点的 HTTP client 实现（实现 domain/mcp_oauth.McpServerClient）。
//
// 装配在 biz/shared/client/；service/workflow 经 domain 接口调用，不直接 import 本包。
// 对齐 control-plane services/mcp-oauth.ts（discover / register / exchange / refresh）。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
	policymcp "media_agent/media_auth/biz/policy/mcp_oauth"
)

// Client 是 MCP server OAuth HTTP 实现。
type Client struct {
	httpClient *http.Client
}

// NewClient 构造 MCP server client。httpClient 为 nil 时用默认 10s 超时。
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{httpClient: httpClient}
}

// DiscoverMetadata GET {origin}/.well-known/oauth-authorization-server。
//
// 返回 (metadata, origin, true, nil) 当 server 支持 OAuth（含 authorize+token endpoint）；
// 不支持（404/非 JSON/缺端点）→ (Metadata{}, "", false, nil)；不可达 → error。
func (c *Client) DiscoverMetadata(ctx context.Context, serverURL string) (domainmcp.Metadata, string, bool, error) {
	origin, err := serverOrigin(serverURL)
	if err != nil {
		return domainmcp.Metadata{}, "", false, fmt.Errorf("mcp: parse server url: %w", err)
	}
	metadataURL := origin + "/.well-known/oauth-authorization-server"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return domainmcp.Metadata{}, "", false, err
	}
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domainmcp.Metadata{}, "", false, fmt.Errorf("mcp: discover %s: %w", metadataURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domainmcp.Metadata{}, "", false, nil // 不支持 OAuth
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "json") {
		return domainmcp.Metadata{}, "", false, nil
	}
	var m domainmcp.Metadata
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return domainmcp.Metadata{}, "", false, nil
	}
	if m.AuthorizationEndpoint == "" || m.TokenEndpoint == "" {
		return domainmcp.Metadata{}, "", false, nil
	}
	return m, origin, true, nil
}

// RegisterClient 动态注册 client（POST registration_endpoint）。
//
// 仅当 metadata 有 registration_endpoint 时尝试；注册成功返回更新后的 client（含 client_id/secret）。
// 注册失败返回 error（service 层 fallback 到无注册流程）。
func (c *Client) RegisterClient(ctx context.Context, mc domainmcp.McpClient, callbackURL string) (domainmcp.McpClient, error) {
	if mc.Metadata.RegistrationEndpoint == "" {
		return mc, fmt.Errorf("mcp: no registration_endpoint for %s", mc.ServerOrigin)
	}
	body := map[string]any{
		"redirect_uris":              []string{callbackURL},
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"client_name":                "media-auth",
	}
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mc.Metadata.RegistrationEndpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return mc, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return mc, fmt.Errorf("mcp: register client: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return mc, fmt.Errorf("mcp: register client -> %d: %s", resp.StatusCode, string(respBody))
	}
	var reg struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&reg); err != nil {
		return mc, fmt.Errorf("mcp: decode register response: %w", err)
	}
	mc.ClientID = reg.ClientID
	mc.ClientSecret = reg.ClientSecret
	return mc, nil
}

// ExchangeCode 用 authorization_code 换 token。
func (c *Client) ExchangeCode(ctx context.Context, mc domainmcp.McpClient, code, codeVerifier, callbackURL string) (domainmcp.TokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {callbackURL},
		"client_id":     {mc.ClientID},
		"code_verifier": {codeVerifier},
	}
	if mc.ClientSecret != "" {
		form.Set("client_secret", mc.ClientSecret)
	}
	return c.postToken(ctx, mc.Metadata.TokenEndpoint, form)
}

// RefreshToken 用 refresh_token 换新 token。
//
// permanent=true 表示 refresh_token/client 已死（4xx + permanent OAuth error 或 4xx 无 error code）；
// 调用方据此走宽限算法。5xx/网络错误 → permanent=false（暂态）。
func (c *Client) RefreshToken(ctx context.Context, mc domainmcp.McpClient, refreshToken string) (domainmcp.TokenResponse, bool, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {mc.ClientID},
	}
	if mc.ClientSecret != "" {
		form.Set("client_secret", mc.ClientSecret)
	}
	resp, err := c.postTokenRaw(ctx, mc.Metadata.TokenEndpoint, form)
	if err != nil {
		return domainmcp.TokenResponse{}, false, err // 网络错误 → 暂态
	}
	if !resp.ok {
		permanent := policymcp.IsPermanentOAuthError(resp.oauthError) ||
			(resp.status >= 400 && resp.status < 500 && resp.oauthError == "")
		return domainmcp.TokenResponse{}, permanent, fmt.Errorf("mcp: refresh failed: %d (%s)", resp.status, resp.oauthError)
	}
	return resp.token, false, nil
}

// ── helpers ──────────────────────────────────────────────────────────

// serverOrigin 从 serverURL 提取 origin（scheme://host[:port]）。
func serverOrigin(serverURL string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid url %q", serverURL)
	}
	return u.Scheme + "://" + u.Host, nil
}

type rawTokenResp struct {
	token      domainmcp.TokenResponse
	ok         bool
	status     int
	oauthError string
}

// postToken POST form 到 token_endpoint，2xx 返回 token。
func (c *Client) postToken(ctx context.Context, endpoint string, form url.Values) (domainmcp.TokenResponse, error) {
	resp, err := c.postTokenRaw(ctx, endpoint, form)
	if err != nil {
		return domainmcp.TokenResponse{}, err
	}
	if !resp.ok {
		return domainmcp.TokenResponse{}, fmt.Errorf("mcp: token endpoint -> %d (%s)", resp.status, resp.oauthError)
	}
	return resp.token, nil
}

// postTokenRaw POST form，解析响应（含 OAuth error code 提取）。
func (c *Client) postTokenRaw(ctx context.Context, endpoint string, form url.Values) (rawTokenResp, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return rawTokenResp{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return rawTokenResp{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	out := rawTokenResp{status: resp.StatusCode}
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(body, &out.token); err != nil {
			return out, fmt.Errorf("mcp: decode token response: %w", err)
		}
		out.ok = true
		return out, nil
	}
	// 非 2xx：尝试解析 OAuth error。
	var oe struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &oe)
	out.oauthError = oe.Error
	return out, nil
}

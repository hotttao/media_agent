// Package wecom 是企微 corp API 的 HTTP client 实现（实现 domain/wecom.WeComClient）。
//
// 装配在 biz/shared/client/（消费 hertz_infra/clienthertz 治理能力）；service/workflow 经
// domain/wecom.WeComClient 接口调用，不直接 import 本包。
//
// corp access_token 内存缓存 2h（1min 安全余量，对齐 control-plane getAccessToken）。
// 对齐 control-plane services/wecom.ts。
package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	domainwecom "media_agent/media_auth/biz/domain/wecom"
)

// Client 是企微 corp API 的 HTTP 实现。
type Client struct {
	cfg        domainwecom.Config
	httpClient *http.Client

	mu             sync.Mutex
	accessToken    string
	tokenExpiresAt time.Time
}

// NewClient 构造企微 client。httpClient 为 nil 时用默认 10s 超时。
func NewClient(cfg domainwecom.Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{cfg: cfg, httpClient: httpClient}
}

// Config 返回当前配置（service 层 GetEnabled 用）。
func (c *Client) Config() domainwecom.Config { return c.cfg }

// getAccessToken 取或刷新 corp access_token（缓存，2h TTL，1min 安全余量）。
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiresAt.Add(-60*time.Second)) {
		return c.accessToken, nil
	}
	// 刷新。
	u := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		url.QueryEscape(c.cfg.CorpID), url.QueryEscape(c.cfg.CorpSecret))
	body, err := c.getJSON(ctx, u)
	if err != nil {
		return "", fmt.Errorf("wecom: gettoken: %w", err)
	}
	var resp struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("wecom: decode gettoken: %w", err)
	}
	if resp.ErrCode != 0 {
		return "", fmt.Errorf("wecom: gettoken failed: %s", resp.ErrMsg)
	}
	c.accessToken = resp.AccessToken
	c.tokenExpiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	return c.accessToken, nil
}

// GetUserIDByCode 用一次性 code 换企微 userid（auth/getuserinfo）。
func (c *Client) GetUserIDByCode(ctx context.Context, code string) (string, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return "", err
	}
	u := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/auth/getuserinfo?access_token=%s&code=%s",
		url.QueryEscape(token), url.QueryEscape(code))
	body, err := c.getJSON(ctx, u)
	if err != nil {
		return "", fmt.Errorf("wecom: getuserinfo: %w", err)
	}
	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		UserID  string `json:"userid"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("wecom: decode getuserinfo: %w", err)
	}
	if resp.ErrCode != 0 || resp.UserID == "" {
		return "", fmt.Errorf("wecom: getuserinfo failed: %s", resp.ErrMsg)
	}
	return resp.UserID, nil
}

// GetUserInfo 按 userid 取档案（user/get）。
func (c *Client) GetUserInfo(ctx context.Context, userid string) (domainwecom.UserInfo, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return domainwecom.UserInfo{}, err
	}
	u := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/user/get?access_token=%s&userid=%s",
		url.QueryEscape(token), url.QueryEscape(userid))
	body, err := c.getJSON(ctx, u)
	if err != nil {
		return domainwecom.UserInfo{}, fmt.Errorf("wecom: user/get: %w", err)
	}
	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		UserID  string `json:"userid"`
		Name    string `json:"name"`
		Avatar  string `json:"avatar"`
		Email   string `json:"email"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return domainwecom.UserInfo{}, fmt.Errorf("wecom: decode user/get: %w", err)
	}
	if resp.ErrCode != 0 {
		return domainwecom.UserInfo{}, fmt.Errorf("wecom: user/get failed: %s", resp.ErrMsg)
	}
	return domainwecom.UserInfo{
		UserID: resp.UserID,
		Name:   resp.Name,
		Avatar: resp.Avatar,
		Email:  resp.Email,
	}, nil
}

// getJSON 发 GET 请求返回 body（非 2xx 返回 error）。
func (c *Client) getJSON(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// Package wecom 存放 wecom（企微 corp OAuth 登录/绑定通道）领域的稳定业务核心：
// Config 值对象、authorize URL 拼装、WeComClient 接口。
//
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM。
// wecom 无持久化表；corp access_token 内存缓存 2h（1min 安全余量），归 shared/client/wecom。
// 登录/绑定流程的跨 domain 编排归 workflow/wecom_login_bind。
package wecom

import (
	"context"
	"net/url"
)

// Config 是企微 corp OAuth 配置（CorpID/CorpSecret/AgentID 三者齐备才启用）。
type Config struct {
	CorpID     string
	CorpSecret string
	AgentID    string
}

// Enabled 三件套齐备才启用。
func (c Config) Enabled() bool {
	return c.CorpID != "" && c.CorpSecret != "" && c.AgentID != ""
}

// AuthorizeURL 拼企微 SSO 扫码授权 URL（login_type=CorpApp）。
//
// 对齐 control-plane getAuthorizeUrl：login.work.weixin.qq.com/wwlogin/sso/login。
// state 即 mode（login|bind），control-plane 现状未引入额外 CSRF 随机态。
func (c Config) AuthorizeURL(redirectURI, state string) string {
	u := &url.URL{
		Scheme: "https",
		Host:   "login.work.weixin.qq.com",
		Path:   "/wwlogin/sso/login",
	}
	q := u.Query()
	q.Set("login_type", "CorpApp")
	q.Set("appid", c.CorpID)
	q.Set("agentid", c.AgentID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}

// CallbackRedirectURI 由 referer/origin 拼回调地址：<origin>/api/auth/wecom/callback。
func CallbackRedirectURI(origin string) string {
	return origin + "/api/auth/wecom/callback"
}

// UserInfo 是企微 user/get 返回的用户档案（bind 流程写 identity.display_name/metadata 用）。
type UserInfo struct {
	UserID string
	Name   string
	Avatar string
	Email  string
}

// WeComClient 是企微 corp API 的稳定接口（由 shared/client/wecom 实现）。
//
// domain 定义接口，实现在 shared（依赖 HTTP client + corp token 缓存）；
// service/workflow 经此接口调用企微，不直接 import HTTP SDK。
type WeComClient interface {
	// GetUserIDByCode 用一次性 code 换企微 userid（auth/getuserinfo）。
	GetUserIDByCode(ctx context.Context, code string) (string, error)
	// GetUserInfo 按 userid 取档案（user/get）。
	GetUserInfo(ctx context.Context, userid string) (UserInfo, error)
}

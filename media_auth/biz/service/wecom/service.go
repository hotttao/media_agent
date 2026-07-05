// Package wecom 是 wecom 用例服务层（GetEnabled / GetAuthorizeUrl；callback 编排归 workflow）。
//
// 接受 command，编排 domain/wecom.Config 完成纯配置用例；禁止 import Hertz / hertz_gen / biz/dal。
// 对齐 control-plane routes/wecom-auth.ts 的 /enabled + /authorize 段。
package wecom

import (
	domainwecom "media_agent/media_auth/biz/domain/wecom"
)

// Service 是 wecom 用例服务（仅配置类用例；callback 编排见 workflow/wecom_login_bind）。
type Service struct {
	cfg domainwecom.Config
}

// NewService 构造 wecom 服务。
func NewService(cfg domainwecom.Config) *Service {
	return &Service{cfg: cfg}
}

// GetEnabled 企微登录是否启用（CorpID/CorpSecret/AgentID 三者齐备）。
func (s *Service) GetEnabled() bool {
	return s.cfg.Enabled()
}

// GetAuthorizeUrl 拼企微 SSO 授权 URL。
//
// origin 来自请求 Referer/Origin（handler 解析后传入）；redirect_uri = origin + /api/auth/wecom/callback。
// mode=login|bind → state=login|bind（control-plane 现状 state 即 mode 字符串）。
// 未配置企微 → error（handler 400）。
func (s *Service) GetAuthorizeUrl(origin, mode string) (string, error) {
	if !s.cfg.Enabled() {
		return "", ErrNotConfigured
	}
	state := "login"
	if mode == "bind" {
		state = "bind"
	}
	redirectURI := domainwecom.CallbackRedirectURI(origin)
	return s.cfg.AuthorizeURL(redirectURI, state), nil
}

// ErrNotConfigured 企微未配置（handler 400）。
var ErrNotConfigured = errSentinel("WeChat Work login is not configured")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

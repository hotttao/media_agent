// Package workflow 存放跨 domain 业务流程编排（architecture.md「biz/workflow」）。
//
// wecom_login_bind：企微 corp OAuth 登录/绑定 callback 的跨 domain 编排——
// wecom client（取 userid/userinfo）+ identity（find/bind）+ auth（user 查询 + system 校验 +
// session 签发）+ 独立密文 platform_token（无Secret确保）。
package workflow

import (
	"context"
	"encoding/json"
	"log"
	"time"

	domainauth "media_agent/media_auth/biz/domain/auth"
	domainwecom "media_agent/media_auth/biz/domain/wecom"
	policyauth "media_agent/media_auth/biz/policy/auth"
	identityservice "media_agent/media_auth/biz/service/identity"
	"media_agent/media_auth/biz/shared/session"
)

// WecomLoginBind 编排企微登录/绑定 callback。
type WecomLoginBind struct {
	wecom    domainwecom.WeComClient
	identity *identityservice.Service
	users    domainauth.UserRepo
	signer   *session.Signer
	ensurer  domainauth.PlatformTokenEnsurer
}

// NewWecomLoginBind 构造企微登录/绑定 workflow。
func NewWecomLoginBind(wecom domainwecom.WeComClient, identity *identityservice.Service, users domainauth.UserRepo, signer *session.Signer, ensurer domainauth.PlatformTokenEnsurer) *WecomLoginBind {
	return &WecomLoginBind{wecom: wecom, identity: identity, users: users, signer: signer, ensurer: ensurer}
}

// WecomCallbackResult 是 HandleCallback 返回。SetCookie=true 时 handler 须下发 SessionToken cookie。
type WecomCallbackResult struct {
	RedirectURL  string
	SessionToken string
	SetCookie    bool
}

// HandleCallback 处理企微回调（login|bind）。
//
// 所有业务异常（缺 code、企微 API 失败、未绑定、system 账号、已绑别人）统一转为 redirect；
// 仅返回 error 表示不可恢复的内部错误（handler 500）。对齐 control-plane try/catch 全 redirect 语义。
//
// 入参 cookieToken：bind 态手动验 cookie 取 sub（callback 在 skip list，middleware 不注入 session）。
func (w *WecomLoginBind) HandleCallback(ctx context.Context, code, state, cookieToken string) (WecomCallbackResult, error) {
	if code == "" {
		return WecomCallbackResult{RedirectURL: "/login?error=missing_code"}, nil
	}

	// 1. code 换企微 userid。
	userid, err := w.wecom.GetUserIDByCode(ctx, code)
	if err != nil {
		log.Printf("wecom: getuserid by code: %v", err)
		return WecomCallbackResult{RedirectURL: "/login?error=oauth_failed"}, nil
	}

	if state == "bind" {
		return w.handleBind(ctx, userid, cookieToken)
	}
	return w.handleLogin(ctx, userid)
}

// handleBind 绑定流程：手动验 cookie → 取企微 userinfo → bindIdentity。
func (w *WecomLoginBind) handleBind(ctx context.Context, userid, cookieToken string) (WecomCallbackResult, error) {
	// callback 在 skip list，middleware 不注入 session；此处手动验 cookie。
	sess, err := w.signer.Verify(cookieToken)
	if err != nil {
		return WecomCallbackResult{RedirectURL: "/login?error=auth_required"}, nil
	}
	// 取企微 userinfo（display_name + metadata）。
	wecomUser, err := w.wecom.GetUserInfo(ctx, userid)
	if err != nil {
		log.Printf("wecom: getuserinfo: %v", err)
		return WecomCallbackResult{RedirectURL: "/login?error=oauth_failed"}, nil
	}
	metadata := buildWecomMetadata(wecomUser)
	if err := w.identity.BindIdentity(ctx, sess.Sub, "wecom", userid, wecomUser.Name, metadata); err != nil {
		// 已绑别人 → already_bound；其它错误 → oauth_failed。
		return WecomCallbackResult{RedirectURL: "/?wecom_bind=already_bound"}, nil
	}
	return WecomCallbackResult{RedirectURL: "/?wecom_bind=success"}, nil
}

// handleLogin 登录流程：findUserIdentity → 查 user → system 校验 → 签 session → ensure platform token。
func (w *WecomLoginBind) handleLogin(ctx context.Context, userid string) (WecomCallbackResult, error) {
	userID, err := w.identity.FindUserIDentity(ctx, "wecom", userid)
	if err != nil {
		log.Printf("wecom: findUserIdentity: %v", err)
		return WecomCallbackResult{RedirectURL: "/login?error=oauth_failed"}, nil
	}
	if userID == "" {
		return WecomCallbackResult{RedirectURL: "/login?error=not_bound"}, nil
	}
	user, err := w.users.FindByID(ctx, userID)
	if err != nil {
		// user 不存在（identity 指向已删 user）。
		return WecomCallbackResult{RedirectURL: "/login?error=user_not_found"}, nil
	}
	if !policyauth.CanLogin(user) {
		return WecomCallbackResult{RedirectURL: "/login?error=system_account"}, nil
	}
	// 签 session。
	sess := domainauth.Session{
		Sub:         user.ID,
		Username:    user.Username,
		Name:        user.DisplayName,
		Email:       user.Email,
		Role:        user.Role,
		AuthVersion: user.AuthVersion,
	}
	token, err := w.signer.Sign(sess)
	if err != nil {
		return WecomCallbackResult{}, err // 不可恢复，handler 500
	}
	// fire-and-forget ensure platform token（失败仅日志）。
	if w.ensurer != nil {
		go func() {
			bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if err := w.ensurer.EnsurePlatformToken(bounded, user.ID); err != nil {
				log.Print("wecom: platform credential bootstrap failed")
			}
		}()
	}
	return WecomCallbackResult{RedirectURL: "/", SessionToken: token, SetCookie: true}, nil
}

// buildWecomMetadata 把企微 avatar/email 组成 jsonb metadata（identity.bindIdentity 存）。
func buildWecomMetadata(u domainwecom.UserInfo) json.RawMessage {
	m := map[string]string{}
	if u.Avatar != "" {
		m["avatar"] = u.Avatar
	}
	if u.Email != "" {
		m["email"] = u.Email
	}
	if len(m) == 0 {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

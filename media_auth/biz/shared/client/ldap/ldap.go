// Package ldap 是 LDAP 客户端封装（外部依赖，归 biz/shared/client 装配层）。
//
// 对齐 control-plane services/auth.ts authenticateLdap：双绑（admin bind+search 找用户 → user bind 验密）。
// 命中返回 LdapUser（dn/name/username/email），否则 nil。
// LDAP 配置来自 config（ldap 段，spec §7）。
package ldap

import (
	"context"
	"fmt"
	"log"

	"github.com/go-ldap/ldap/v3"

	configpb "media_agent/hertz_gen/config"
)

// LdapUser 是 LDAP 认证命中后提取的用户信息（对齐 control-plane LdapUser）。
type LdapUser struct {
	DN       string
	Name     string
	Username string
	Email    string
}

// Client 是 LDAP 客户端。enabled=false 时 Authenticate 恒返回 nil（未启用）。
type Client struct {
	enabled bool
	cfg     *configpb.LdapConfig
}

// New 按 LdapConfig 构造 Client。enabled=false 返回禁用实例（Authenticate 恒 nil）。
func New(cfg *configpb.LdapConfig) *Client {
	if cfg == nil {
		return &Client{enabled: false}
	}
	return &Client{enabled: cfg.GetEnabled(), cfg: cfg}
}

// Authenticate 双绑认证：admin bind 搜索用户 → user bind 验密。
// 未启用 / 未找到用户 / 密码错 → (nil, nil)；连接错误 → error。
func (c *Client) Authenticate(ctx context.Context, username, password string) (*LdapUser, error) {
	if !c.enabled {
		return nil, nil
	}

	l, err := ldap.DialURL(c.cfg.GetUrl())
	if err != nil {
		return nil, fmt.Errorf("ldap: dial %s: %w", c.cfg.GetUrl(), err)
	}
	defer l.Close()

	// 1. admin bind 搜索用户。
	if err := l.Bind(c.cfg.GetBindDn(), c.cfg.GetBindPassword()); err != nil {
		return nil, fmt.Errorf("ldap: admin bind: %w", err)
	}

	filter := fmt.Sprintf("(&%s(%s=%s))", c.cfg.GetSearchFilter(), c.cfg.GetAttrUsername(), ldap.EscapeFilter(username))
	searchReq := ldap.NewSearchRequest(
		c.cfg.GetSearchBase(),
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		[]string{c.cfg.GetAttrName(), c.cfg.GetAttrUsername(), c.cfg.GetAttrEmail()},
		nil,
	)
	sr, err := l.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("ldap: search: %w", err)
	}
	if len(sr.Entries) == 0 {
		return nil, nil // 未找到用户
	}
	entry := sr.Entries[0]
	userDN := entry.DN

	// 2. user bind 验密（unbind admin 后用新连接或复用？control-plane 用新 client；这里 unbind 后重新 bind）。
	if err := l.Bind(userDN, password); err != nil {
		// bind 失败 = 密码错（LDAP invalid credentials），返回 nil 非 error。
		log.Printf("ldap: invalid credentials for %s: %v", username, err)
		return nil, nil
	}

	return &LdapUser{
		DN:       userDN,
		Name:     entry.GetAttributeValue(c.cfg.GetAttrName()),
		Username: entry.GetAttributeValue(c.cfg.GetAttrUsername()),
		Email:    entry.GetAttributeValue(c.cfg.GetAttrEmail()),
	}, nil
}

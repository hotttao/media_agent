// user_test.go：auth 领域 User 实体单测。
//
// 覆盖：HasPassword、AuthSource 推导（password / wecom / ldap 回退）。
package auth

import (
	"context"
	"testing"
)

// fakeResolver 实现 AuthSourceResolver，按 userID 返回固定 provider（模拟 identity domain）。
type fakeResolver struct {
	byUser map[string]string // userID → provider
}

func (r fakeResolver) ResolveByUserID(_ context.Context, userID string) string {
	return r.byUser[userID]
}

func TestUserHasPassword(t *testing.T) {
	if (User{PasswordHash: "x"}).HasPassword() != true {
		t.Errorf("HasPassword should be true when hash set")
	}
	if (User{}).HasPassword() != false {
		t.Errorf("HasPassword should be false when hash empty")
	}
}

func TestUserAuthSource(t *testing.T) {
	resolver := fakeResolver{byUser: map[string]string{"u_wecom": "wecom"}}
	cases := []struct {
		name     string
		user     User
		resolver AuthSourceResolver
		want     string
	}{
		{"有 password_hash → password", User{ID: "u1", PasswordHash: "x"}, resolver, "password"},
		{"无 hash + wecom identity → wecom（修正 control-plane 误报 ldap）", User{ID: "u_wecom"}, resolver, "wecom"},
		{"无 hash + 无 identity → ldap（回退）", User{ID: "u_ldap"}, resolver, "ldap"},
		{"无 hash + resolver 为 nil → ldap（identity 未注入桩）", User{ID: "u_ldap"}, nil, "ldap"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.user.AuthSource(c.resolver); got != c.want {
				t.Errorf("AuthSource = %q, want %q", got, c.want)
			}
		})
	}
}

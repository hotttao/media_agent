// policy_test.go：team 领域纯决策规则单测（不查 DB，不 import dal）。
//
// 覆盖：末位 admin 保护（降级/移除）、role 合法性、邀请过期判定。
package team

import (
	"errors"
	"testing"
	"time"

	domainteam "media_agent/media_auth/biz/domain/team"
)

func TestIsValidRole(t *testing.T) {
	cases := []struct {
		role string
		want bool
	}{
		{"admin", true},
		{"member", true},
		{"", false},
		{"owner", false},
		{"ADMIN", false}, // 大小写敏感
	}
	for _, c := range cases {
		if got := IsValidRole(c.role); got != c.want {
			t.Errorf("IsValidRole(%q) = %v, want %v", c.role, got, c.want)
		}
	}
}

func TestCanDemoteMember_LastAdminProtection(t *testing.T) {
	// 末位 admin 降级 → 拒绝；多 admin 时允许；升 admin 不触发保护。
	cases := []struct {
		name          string
		targetCurrent string
		targetNew     string
		adminCount    int
		wantErr       bool
		wantErrIs     error
	}{
		{"末位 admin 降级 → 拒绝", RoleAdmin, RoleMember, 1, true, ErrCannotDemoteLastAdmin},
		{"多 admin 降级 → 允许", RoleAdmin, RoleMember, 2, false, nil},
		{"member 降级 member → 允许", RoleMember, RoleMember, 1, false, nil},
		{"member 升 admin → 允许（不触发保护）", RoleMember, RoleAdmin, 1, false, nil},
		{"admin 升 admin → 允许", RoleAdmin, RoleAdmin, 1, false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CanDemoteMember(c.targetCurrent, c.targetNew, c.adminCount)
			if c.wantErr {
				if !errors.Is(err, c.wantErrIs) {
					t.Errorf("want err %v, got %v", c.wantErrIs, err)
				}
			} else if err != nil {
				t.Errorf("want nil, got %v", err)
			}
		})
	}
}

func TestCanRemoveMember_LastAdminProtection(t *testing.T) {
	// 末位 admin 移除 → 拒绝；多 admin 移除 → 允许；member 移除 → 允许。
	cases := []struct {
		name          string
		targetCurrent string
		adminCount    int
		wantErr       bool
	}{
		{"末位 admin 移除 → 拒绝", RoleAdmin, 1, true},
		{"多 admin 移除 → 允许", RoleAdmin, 2, false},
		{"member 移除 → 允许", RoleMember, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CanRemoveMember(c.targetCurrent, c.adminCount)
			if c.wantErr && !errors.Is(err, ErrCannotRemoveLastAdmin) {
				t.Errorf("want ErrCannotRemoveLastAdmin, got %v", err)
			}
			if !c.wantErr && err != nil {
				t.Errorf("want nil, got %v", err)
			}
		})
	}
}

func TestIsExpired(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		expiresAt *time.Time
		want      bool
	}{
		{"nil（不过期）→ false", nil, false},
		{"未来 → false", timePtr(now.Add(time.Hour)), false},
		{"恰好现在 → true（<= now 即过期）", timePtr(now), true},
		{"过去 → true", timePtr(now.Add(-time.Hour)), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsExpired(c.expiresAt, now); got != c.want {
				t.Errorf("IsExpired = %v, want %v", got, c.want)
			}
		})
	}
}

// 编译期断言：RoleAdmin/RoleMember 与 domain 一致。
var _ = domainteam.RoleAdmin

func timePtr(t time.Time) *time.Time { return &t }

// policy_test.go：mcp_oauth 领域纯决策规则单测（不查 DB）。
//
// 覆盖：刷新失败宽限算法（ShouldDropToken）、permanent OAuth error 分类。
package mcpoauth

import (
	"testing"
	"time"

	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
)

func TestIsPermanentOAuthError(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"invalid_grant", true},
		{"invalid_client", true},
		{"unauthorized_client", true},
		{"invalid_request", false},
		{"server_error", false},
		{"temporarily_unavailable", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsPermanentOAuthError(c.code); got != c.want {
			t.Errorf("IsPermanentOAuthError(%q) = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestShouldDropToken_GraceAlgorithm(t *testing.T) {
	// 宽限算法：failCount >= 3 且 firstFailAt 距今 >= 600s 才删。
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	graceAgo := now.Add(-domainmcp.RefreshFailGraceSeconds * time.Second) // 恰好 600s 前
	shortAgo := now.Add(-300 * time.Second)                               // 300s 前（窗口内）

	cases := []struct {
		name        string
		failCount   int
		firstFailAt *time.Time
		want        bool
	}{
		{"nil firstFailAt → false（不应发生）", 5, nil, false},
		{"次数不足（2 次）且超窗口 → false", 2, &graceAgo, false},
		{"次数足够（3 次）但窗口内（300s）→ false", 3, &shortAgo, false},
		{"次数足够（3 次）且超窗口（600s）→ true", 3, &graceAgo, true},
		{"次数远超（10 次）且超窗口 → true", 10, &graceAgo, true},
		{"次数足够（3 次）恰 600s 前 → true（>= 边界）", 3, &graceAgo, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ShouldDropToken(c.failCount, c.firstFailAt, now); got != c.want {
				t.Errorf("ShouldDropToken = %v, want %v", got, c.want)
			}
		})
	}
}

// 边界：刚好 599s 不应删，刚好 600s 应删（与 3 次组合）。
func TestShouldDropToken_Boundary(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	justUnder := now.Add(-599 * time.Second)
	atBoundary := now.Add(-600 * time.Second)
	if ShouldDropToken(3, &justUnder, now) {
		t.Errorf("599s 不应删")
	}
	if !ShouldDropToken(3, &atBoundary, now) {
		t.Errorf("600s 应删")
	}
}

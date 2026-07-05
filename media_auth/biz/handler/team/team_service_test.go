// team_service_test.go：team handler 错误→HTTP 状态码映射测试（ut.CreateUtRequestContext，不走网络）。
//
// 覆盖 writeTeamError 的非平凡映射：400(参数/user不存在) / 403(非 admin) /
// 404(team/成员/邀请不存在) / 409(末位 admin) / 410(邀请过期) / 500(其它)。
// 业务规则断言在 policy/service 测试；此处仅断言 HTTP 映射。
package team

import (
	"errors"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	teamservice "media_agent/media_auth/biz/service/team"
)

func TestWriteTeamError_StatusMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"invalid name → 400", teamservice.ErrInvalidName, 400},
		{"invalid role → 400", teamservice.ErrInvalidRole, 400},
		{"user not found → 400", teamservice.ErrUserNotFound, 400},
		{"forbidden (non-admin) → 403", teamservice.ErrForbidden, 403},
		{"team not found → 404", teamservice.ErrTeamNotFound, 404},
		{"not member → 404", teamservice.ErrNotMember, 404},
		{"invite not found → 404", teamservice.ErrInviteNotFound, 404},
		{"cannot demote last admin → 409", teamservice.ErrCannotDemoteLast, 409},
		{"cannot remove last admin → 409", teamservice.ErrCannotRemoveLast, 409},
		{"invite expired → 410", teamservice.ErrInviteExpired, 410},
		{"unknown error → 500", errors.New("something broke"), 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := ut.CreateUtRequestContext("GET", "/api/teams/x", nil)
			writeTeamError(ctx, c.err)
			if ctx.Response.StatusCode() != c.wantStatus {
				t.Errorf("writeTeamError(%v): status got %d, want %d", c.err, ctx.Response.StatusCode(), c.wantStatus)
			}
		})
	}
}

// 验证 404 不暴露存在性（统一 "not found" 文案）。
func TestWriteTeamError_NotFoundNoLeak(t *testing.T) {
	ctx := ut.CreateUtRequestContext("GET", "/api/teams/x", nil)
	writeTeamError(ctx, teamservice.ErrNotMember)
	if ctx.Response.StatusCode() != consts.StatusNotFound {
		t.Fatalf("status: got %d, want 404", ctx.Response.StatusCode())
	}
	body := string(ctx.Response.Body())
	if !contains(body, "not found") {
		t.Errorf("404 body should be generic 'not found', got %s", body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

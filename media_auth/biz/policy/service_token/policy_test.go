// policy_test.go：service_token 领域纯决策规则单测（不查 DB）。
//
// 覆盖：CanRevoke（platform 不可撤销 / 仅本人 / 他人 token 不可撤销）。
package servicetoken

import (
	"testing"

	domainst "media_agent/media_auth/biz/domain/service_token"
)

func TestCanRevoke(t *testing.T) {
	ownerID := "u1"
	cases := []struct {
		name    string
		token   domainst.Token
		userID  string
		wantErr bool
	}{
		{"本人非 platform token → 允许", domainst.Token{ID: "t1", CreatedBy: ownerID, IsPlatform: false}, ownerID, false},
		{"platform token → 拒绝（不可撤销）", domainst.Token{ID: "t2", CreatedBy: ownerID, IsPlatform: true}, ownerID, true},
		{"他人 token → 拒绝（不属本人，统一 not found）", domainst.Token{ID: "t3", CreatedBy: "u2", IsPlatform: false}, ownerID, true},
		{"他人 platform token → 拒绝（platform 优先判）", domainst.Token{ID: "t4", CreatedBy: "u2", IsPlatform: true}, ownerID, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CanRevoke(c.token, c.userID)
			if c.wantErr && err == nil {
				t.Errorf("want error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Errorf("want nil, got %v", err)
			}
		})
	}
}

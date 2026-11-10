package chat_test

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"media_agent/hertz_infra/authn/appcredential"
	handler "media_agent/media_agent_gateway/biz/handler/chat"
	router "media_agent/media_agent_gateway/biz/router/chat"
	"media_agent/media_agent_gateway/biz/workflow"
	"strings"
	"testing"
)

type deniedAuthority struct{ calls int }

func (a *deniedAuthority) Verify(_ context.Context, _ string, scope string) (appcredential.Principal, error) {
	a.calls++
	if scope != "session.chat" {
		panic("unexpected scope")
	}
	return appcredential.Principal{}, appcredential.ErrDenied
}
func TestApplicationRoutesRequireDedicatedAuthorityDespiteCookie(t *testing.T) {
	authority := &deniedAuthority{}
	handler.ConfigureApplications(&workflow.ApplicationChat{Authority: authority})
	defer handler.ConfigureApplications(nil)
	h := server.New()
	router.Register(h)
	tests := []struct{ method, path, body string }{
		{"POST", "/apps/v1/chat/sessions", `{"workspace_id":"workspace","title":"test"}`},
		{"GET", "/apps/v1/chat/sessions?workspace_id=workspace", ""},
		{"GET", "/apps/v1/chat/sessions/session/messages", ""},
		{"POST", "/apps/v1/chat/sessions/session/turns", `{"message":"hello","idempotency_key":"key"}`},
		{"POST", "/apps/v1/chat/sessions/session/interrupt", `{}`},
		{"GET", "/apps/v1/chat/sessions/session/events", ""},
		{"GET", "/apps/v1/chat/sessions/session/requests", ""},
		{"POST", "/apps/v1/chat/sessions/session/turns/turn/requests/request/resolve", `{}`},
	}
	for _, tc := range tests {
		r := ut.PerformRequest(h.Engine, tc.method, tc.path, &ut.Body{Body: strings.NewReader(tc.body), Len: len(tc.body)}, ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Cookie", Value: "token=browser-session"}, ut.Header{Key: "Authorization", Value: "Bearer invalid"})
		if r.Code != 403 {
			t.Fatalf("%s %s: %d", tc.method, tc.path, r.Code)
		}
	}
	if authority.calls != len(tests) {
		t.Fatal("application route skipped its authority")
	}
}

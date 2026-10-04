package agent_identity

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	domain "media_agent/media_control/biz/domain/workspace"
	"media_agent/media_control/biz/middleware"
	service "media_agent/media_control/biz/service/agent_identity"
)

type resolverStub struct {
	value *domain.AgentIdentity
	err   error
	calls int
}

func (r *resolverStub) Resolve(context.Context, string) (*domain.AgentIdentity, error) {
	r.calls++
	return r.value, r.err
}

func TestResolveIdentityInternalBoundaryAndSafeErrors(t *testing.T) {
	previous := resolver
	t.Cleanup(func() { resolver = previous; middleware.Configure("", "") })
	middleware.Configure("test-jwt", "internal-secret")
	stub := &resolverStub{value: &domain.AgentIdentity{ConfigurationID: "config", WorkspaceID: "workspace", OwnerUserID: "owner", OrganizationID: "org", TeamID: "team", WorkspaceState: "running", OwnerAuthorized: true}}
	Configure(stub)
	h := server.New()
	h.GET("/internal/v1/agent-identities/:agent_configuration_id", middleware.Session(), middleware.RequireInternalSystem(), ResolveAgentIdentity)
	for _, tc := range []struct {
		name, token, role string
		want              int
	}{
		{"no service credential", "", "system", 401},
		{"invalid credential", "wrong", "system", 401},
		{"non-system service", "internal-secret", "user", 403},
		{"system service", "internal-secret", "system", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := stub.calls
			w := ut.PerformRequest(h.Engine, "GET", "/internal/v1/agent-identities/config", nil,
				ut.Header{Key: "X-Control-Service-Token", Value: tc.token}, ut.Header{Key: "X-Control-Acting-User-ID", Value: "media-auth"}, ut.Header{Key: "X-Control-Acting-Role", Value: tc.role})
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d", w.Code, tc.want)
			}
			if tc.want != 200 && stub.calls != before {
				t.Fatal("unauthorized request reached identity lookup")
			}
			if tc.want == 200 && !bytes.Contains(w.Body.Bytes(), []byte(`"actor_id":"Agent:config"`)) {
				t.Fatal("stable actor missing")
			}
		})
	}
	for _, tc := range []struct {
		err  error
		want int
	}{
		{domain.ErrNotFound, 404}, {service.ErrInvalidID, 400}, {service.ErrIdentityUnavailable, 503}, {errors.New("provider-secret-and-sql-diagnostics"), 503},
	} {
		stub.err = tc.err
		w := ut.PerformRequest(h.Engine, "GET", "/internal/v1/agent-identities/config", nil,
			ut.Header{Key: "X-Control-Service-Token", Value: "internal-secret"}, ut.Header{Key: "X-Control-Acting-User-ID", Value: "media-auth"}, ut.Header{Key: "X-Control-Acting-Role", Value: "system"})
		if w.Code != tc.want || bytes.Contains(w.Body.Bytes(), []byte("provider-secret")) {
			t.Fatalf("unsafe error response: status=%d", w.Code)
		}
	}
}

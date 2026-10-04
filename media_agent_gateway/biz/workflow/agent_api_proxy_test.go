package workflow

import (
	"context"
	"errors"
	"testing"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type proxyFixture struct {
	principal                             internaljwt.AgentPrincipal
	facts                                 client.AgentIdentity
	verifyErr                             error
	verified, resolved, signed, forwarded int
	assertionPrincipal                    internaljwt.AgentPrincipal
	audience                              string
}

func (f *proxyFixture) VerifyAgentCredential(context.Context, string) (internaljwt.AgentPrincipal, error) {
	f.verified++
	return f.principal, f.verifyErr
}
func (f *proxyFixture) ResolveAgentIdentity(context.Context, string) (client.AgentIdentity, error) {
	f.resolved++
	return f.facts, nil
}
func (f *proxyFixture) Sign(p internaljwt.AgentPrincipal, a string, _ internaljwt.AgentRequest) (string, error) {
	f.signed++
	f.assertionPrincipal = p
	f.audience = a
	return "internal-assertion", nil
}
func (f *proxyFixture) ForwardAgentRequest(ctx context.Context, _ AgentProxyRequest, assertion string) (AgentProxyResponse, error) {
	f.forwarded++
	if assertion != "internal-assertion" {
		return AgentProxyResponse{}, errors.New("unexpected assertion")
	}
	p, ok := internaljwt.AgentPrincipalFromContext(ctx)
	if !ok || p != f.assertionPrincipal {
		return AgentProxyResponse{}, errors.New("identity missing from forwarding context")
	}
	return AgentProxyResponse{StatusCode: 200, Body: []byte(`{"ok":true}`)}, nil
}
func newProxyFixture() *proxyFixture {
	return &proxyFixture{
		principal: internaljwt.AgentPrincipal{UserID: "owner", ActorID: "Agent:config", OrganizationID: "org", CredentialID: "credential", WorkspaceID: "untrusted"},
		facts:     client.AgentIdentity{ConfigurationID: "config", ActorID: "Agent:config", OwnerUserID: "owner", OrganizationID: "org", WorkspaceID: "workspace", TeamID: "team", State: "stopped", OwnerAuthorized: true},
	}
}
func TestAgentProxyUsesCurrentControlFacts(t *testing.T) {
	f := newProxyFixture()
	w := NewAgentAPIProxy(f, f, f, f)
	request := AgentProxyRequest{Service: "workflow", Request: internaljwt.AgentRequest{Method: "GET", URI: "/api/v1/jobs?status=running"}}
	response, err := w.Execute(context.Background(), "raw-api-credential", request)
	if err != nil || response.StatusCode != 200 || f.verified != 1 || f.resolved != 1 || f.signed != 1 || f.forwarded != 1 {
		t.Fatal("delegated request did not complete the verified pipeline", err)
	}
	if f.assertionPrincipal.WorkspaceID != "workspace" || f.audience != "media-workflow-agent" {
		t.Fatal("authority or audience was not fixed by the gateway")
	}
}
func TestAgentProxyDeniesChangedAuthorityBeforeSigning(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*proxyFixture)
	}{
		{"worker identity", func(f *proxyFixture) { f.principal.ActorID = "Worker:config" }},
		{"owner changed", func(f *proxyFixture) { f.facts.OwnerUserID = "other" }},
		{"organization changed", func(f *proxyFixture) { f.facts.OrganizationID = "other" }},
		{"deleted", func(f *proxyFixture) { f.facts.State = "deleted" }},
		{"owner permission removed", func(f *proxyFixture) { f.facts.OwnerAuthorized = false }},
		{"workspace removed", func(f *proxyFixture) { f.facts.WorkspaceID = "" }},
		{"actor mismatch", func(f *proxyFixture) { f.facts.ActorID = "Agent:other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProxyFixture()
			tc.change(f)
			_, err := NewAgentAPIProxy(f, f, f, f).Execute(context.Background(), "credential", AgentProxyRequest{Service: "materials", Request: internaljwt.AgentRequest{Method: "GET", URI: "/api/v1/materials"}})
			if !errors.Is(err, ErrAgentProxyForbidden) || f.signed != 0 || f.forwarded != 0 {
				t.Fatal("unauthorized request reached signing or forwarding", err)
			}
		})
	}
	f := newProxyFixture()
	f.verifyErr = errors.New("revoked")
	_, err := NewAgentAPIProxy(f, f, f, f).Execute(context.Background(), "credential", AgentProxyRequest{Service: "materials", Request: internaljwt.AgentRequest{Method: "GET", URI: "/api/v1/materials"}})
	if err == nil || f.resolved != 0 || f.signed != 0 || f.forwarded != 0 {
		t.Fatal("invalid credential reached authority resolution")
	}
}
func TestAgentProxyRouteAllowlist(t *testing.T) {
	for _, tc := range []struct {
		service, method, uri string
		allowed              bool
	}{
		{"workflow", "GET", "/api/v1/jobs?limit=5", true},
		{"workflow", "POST", "/api/v1/step-runs/step/confirm", true},
		{"workflow", "POST", "/api/v1/workflows/validate", true},
		{"workflow", "GET", "/api/v1/templates/tpl/versions/1", true},
		{"materials", "PUT", "/api/v1/products/product", true},
		{"materials", "GET", "/api/v1/asset-kinds", true},
		{"materials", "POST", "/api/v1/library-objects:upload", true},
		{"workflow", "POST", "/api/v1/step-runs/claim", false},
		{"workflow", "POST", "/api/v1/step-runs/step/complete", false},
		{"workflow", "POST", "/api/v1/step-runs/step/fail", false},
		{"materials", "POST", "/api/v1/asset-kinds", false},
		{"materials", "DELETE", "/api/v1/asset-kinds/kind", false},
		{"materials", "GET", "/api/internal/v1/generated-assets/import/id", false},
		{"workflow", "GET", "https://outside.test/api/v1/jobs", false},
		{"workflow", "GET", "//outside.test/api/v1/jobs", false},
		{"workflow", "GET", "/api/v1/jobs/../jobs", false},
		{"workflow", "GET", "/api/v1/%6aobs", false},
		{"workflow", "GET", "/api/v1/jobs//id", false},
		{"workflow", "GET", "/api/v1/jobs#fragment", false},
		{"auth", "GET", "/api/v1/jobs", false},
		{"materials", "CONNECT", "/api/v1/materials", false},
	} {
		t.Run(tc.service+":"+tc.method+":"+tc.uri, func(t *testing.T) {
			r := AgentProxyRequest{Service: tc.service, Request: internaljwt.AgentRequest{Method: tc.method, URI: tc.uri}}
			_, ok := agentProxyAudience(r)
			if ok != tc.allowed {
				t.Fatalf("allowed=%v want %v", ok, tc.allowed)
			}
			if !ok {
				f := newProxyFixture()
				_, err := NewAgentAPIProxy(f, f, f, f).Execute(context.Background(), "credential", r)
				if !errors.Is(err, ErrAgentProxyInvalid) || f.verified != 0 {
					t.Fatal("invalid route reached credential verification")
				}
			}
		})
	}
}

func TestAgentProxyMultipartRequiresValidBoundary(t *testing.T) {
	for _, tc := range []struct {
		contentType string
		allowed     bool
	}{
		{"multipart/form-data", false},
		{"multipart/form-data; boundary=fixture", true},
		{"multipart/form-data; boundary=\"\"", false},
		{"multipart/form-data; boundary=\"invalid@boundary\"", false},
	} {
		r := AgentProxyRequest{Service: "materials", Request: internaljwt.AgentRequest{Method: "POST", URI: "/api/v1/library-objects:upload", ContentType: tc.contentType}}
		_, ok := agentProxyAudience(r)
		if ok != tc.allowed {
			t.Fatalf("Content-Type %q allowed=%v want=%v", tc.contentType, ok, tc.allowed)
		}
	}
}

package agent_proxy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/workflow"
)

func TestUnconfiguredAgentEndpointsFailClosed(t *testing.T) {
	Configure(nil, nil)
	for _, tc := range []struct {
		name    string
		handler func(context.Context, *app.RequestContext)
	}{
		{"proxy", ProxyAgentRequest},
		{"jwks", GetAgentJWKS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := app.NewContext(0)
			tc.handler(context.Background(), c)
			if c.Response.StatusCode() != 503 {
				t.Fatal("unconfigured endpoint reported success")
			}
			if string(c.Response.Header.Peek("Cache-Control")) != "no-store" {
				t.Fatal("unavailable response could be cached")
			}
			if !strings.Contains(string(c.Response.Body()), "unavailable") {
				t.Fatal("unavailable response lacks a safe error")
			}
		})
	}
}

type fakeExecutor struct {
	err        error
	called     bool
	credential string
	request    workflow.AgentProxyRequest
}

func (e *fakeExecutor) Execute(_ context.Context, credential string, request workflow.AgentProxyRequest) (workflow.AgentProxyResponse, error) {
	e.called = true
	e.credential = credential
	e.request = request
	return workflow.AgentProxyResponse{StatusCode: 409, ContentType: "application/json", Body: []byte(`{"error":"conflict"}`), TraceID: "trace"}, e.err
}
func TestConfiguredAgentProxyHTTPContract(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := internaljwt.NewAgentSigner(key, "urn:media:agent-gateway", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, header, body string
		failure            error
		status             int
		called             bool
	}{
		{"no bearer", "", `{}`, nil, 401, false},
		{"cookie is not authentication", "", `{}`, nil, 401, false},
		{"invalid envelope", "Bearer credential", `{`, nil, 400, false},
		{"invalid route", "Bearer credential", `{}`, workflow.ErrAgentProxyInvalid, 400, true},
		{"revoked", "Bearer credential", `{}`, client.ErrAgentCredentialRejected, 401, true},
		{"forbidden", "Bearer credential", `{}`, workflow.ErrAgentProxyForbidden, 403, true},
		{"deleted configuration", "Bearer credential", `{}`, client.ErrAgentTokenNotFound, 403, true},
		{"unavailable", "Bearer credential", `{}`, errors.New("private-upstream-secret"), 503, true},
		{"business response", "Bearer credential", `{"service":"workflow","method":"GET","uri":"/api/v1/jobs"}`, nil, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeExecutor{err: tc.failure}
			Configure(e, signer)
			t.Cleanup(func() { Configure(nil, nil) })
			h := server.New()
			h.POST("/api/agent/v1/requests", ProxyAgentRequest)
			response := ut.PerformRequest(h.Engine, "POST", "/api/agent/v1/requests", &ut.Body{Body: strings.NewReader(tc.body), Len: len(tc.body)}, ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Authorization", Value: tc.header}, ut.Header{Key: "Cookie", Value: "ory_session=ignored"})
			if response.Code != tc.status || e.called != tc.called {
				t.Fatal("unexpected authentication or response status")
			}
			if strings.Contains(response.Body.String(), "private-upstream-secret") {
				t.Fatal("upstream error leaked")
			}
			if tc.status == 200 {
				var envelope struct {
					StatusCode int    `json:"status_code"`
					Body       []byte `json:"body"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.StatusCode != 409 || string(envelope.Body) != `{"error":"conflict"}` || e.credential != "credential" || e.request.Service != "workflow" {
					t.Fatal("proxy response or command mapping lost")
				}
			}
		})
	}
	Configure(&fakeExecutor{}, signer)
	defer Configure(nil, nil)
	c := app.NewContext(0)
	GetAgentJWKS(context.Background(), c)
	if c.Response.StatusCode() != 200 || !strings.Contains(string(c.Response.Body()), `"kid"`) {
		t.Fatal("public signing keys unavailable")
	}
	for _, private := range []string{`"d":`, `"p":`, `"q":`, `PRIVATE KEY`} {
		if strings.Contains(string(c.Response.Body()), private) {
			t.Fatal("private signing material exposed")
		}
	}
}

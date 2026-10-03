package agent_token

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	api "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
	"media_agent/media_auth/biz/shared/client/control"
	"media_agent/media_auth/biz/workflow"
)

type provisioningStub struct {
	command workflow.CreateAgentTokenCommand
	err     error
	calls   int
}

func (s *provisioningStub) Create(_ context.Context, cmd workflow.CreateAgentTokenCommand) (service.CreateResult, error) {
	s.calls++
	s.command = cmd
	return service.CreateResult{Token: api.Token{ID: "token", ActorType: api.ActorAgent, ActorID: "Agent:config", SubjectUserID: "owner", OrganizationID: "org", Scopes: []string{"agent.gateway"}}, Secret: "one-time-secret"}, s.err
}

func TestGatewayCredentialAndErrorBoundary(t *testing.T) {
	previous, previousToken := provisioner, gatewayToken
	t.Cleanup(func() { provisioner, gatewayToken = previous, previousToken })
	stub := &provisioningStub{}
	Configure(stub, "dedicated-secret")
	h := server.New()
	h.POST("/internal/v1/agent-api-tokens", CreateActorAPIToken)
	body := `{"agent_configuration_id":"config","name":"agent","expires_at":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `","idempotency_key":"once","owner_user_id":"forged","actor_id":"Worker:forged","organization_id":"forged"}`
	request := func(token, user string) *ut.ResponseRecorder {
		return ut.PerformRequest(h.Engine, "POST", "/internal/v1/agent-api-tokens", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "X-Agent-Gateway-Internal-Token", Value: token}, ut.Header{Key: "X-Media-Auth-Acting-User-ID", Value: user})
	}
	for _, tc := range []struct{ token, user string }{{"", "owner"}, {"general-internal-secret", "owner"}, {"dedicated-secret", ""}} {
		w := request(tc.token, tc.user)
		if w.Code != 401 || stub.calls != 0 {
			t.Fatal("untrusted caller reached provisioning")
		}
	}
	w := request("dedicated-secret", "owner")
	if w.Code != 201 || stub.command.RequesterUserID != "owner" || stub.command.ConfigurationID != "config" || bytes.Contains(w.Body.Bytes(), []byte("forged")) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("trusted command/one-time response mismatch")
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{service.ErrInvalidRequest, 400}, {service.ErrForbidden, 403}, {control.ErrNotFound, 404}, {api.ErrConflict, 409}, {errors.New("unsafe-provider-secret"), 503}} {
		stub.err = tc.err
		w := request("dedicated-secret", "owner")
		if w.Code != tc.status || bytes.Contains(w.Body.Bytes(), []byte("one-time-secret")) || bytes.Contains(w.Body.Bytes(), []byte("unsafe-provider-secret")) {
			t.Fatal("unsafe error response")
		}
	}
	Configure(stub, "")
	if request("dedicated-secret", "owner").Code != 401 {
		t.Fatal("unconfigured credential failed open")
	}
}

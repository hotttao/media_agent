package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	hconfig "github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/protocol"
	tokenclient "media_agent/hertz_gen/media_auth/token_service"
	tokenpb "media_agent/hertz_gen/model/media_auth/token"
	"media_agent/hertz_infra/authn/internaljwt"
)

var ErrAgentCredentialRejected = errors.New("Agent credential rejected")

type AgentCredentialClient struct{ auth tokenclient.Client }

func NewAgentCredentialClient(authURL, internalCredential string) (*AgentCredentialClient, error) {
	u, err := url.Parse(authURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || internalCredential == "" || strings.ContainsAny(internalCredential, "\r\n") {
		return nil, ErrAgentTokenUnavailable
	}
	auth, err := tokenclient.NewTokenServiceClient(strings.TrimRight(authURL, "/"), tokenclient.WithResponseResultDecider(func(int, *protocol.Response) bool { return false }), tokenclient.WithHeader(http.Header{"X-Media-Auth-Internal-Token": {internalCredential}}), tokenclient.WithHertzClientMiddleware(agentTokenHeaders()))
	if err != nil {
		return nil, ErrAgentTokenUnavailable
	}
	return &AgentCredentialClient{auth: auth}, nil
}

// Introspect every request: no local credential cache or Cookie fallback.
// Generated client errors may contain response bodies; never expose them.
func (c *AgentCredentialClient) VerifyAgentCredential(ctx context.Context, raw string) (internaljwt.AgentPrincipal, error) {
	if len(raw) == 0 || len(raw) > 8192 || strings.ContainsAny(raw, " \t\r\n\x00") {
		return internaljwt.AgentPrincipal{}, ErrAgentCredentialRejected
	}
	if c == nil || c.auth == nil {
		return internaljwt.AgentPrincipal{}, ErrAgentTokenUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, agentTokenHeadersKey{}, traceHeaders(ctx))
	result, response, err := c.auth.VerifyApiToken(ctx, &tokenpb.VerifyApiTokenRequest{Token: raw}, hconfig.WithRequestTimeout(5*time.Second))
	if err != nil || response == nil || response.StatusCode() != http.StatusOK || result == nil {
		return internaljwt.AgentPrincipal{}, ErrAgentTokenUnavailable
	}
	user, userOK := strings.CutPrefix(result.Sub, "User:")
	actor, actorOK := strings.CutPrefix(result.ActorId, "Agent:")
	if !result.Active || result.ActorType != "agent" || !userOK || !actorOK || !validAgentIdentityComponent(user) || !validAgentIdentityComponent(actor) || !validAgentIdentityComponent(result.OrganizationId) || !validAgentIdentityComponent(result.CredentialId) || result.AuthSource != "api_token" || result.Scope != "agent.gateway" || result.Exp <= time.Now().Unix() {
		return internaljwt.AgentPrincipal{}, ErrAgentCredentialRejected
	}
	return internaljwt.AgentPrincipal{UserID: user, ActorID: result.ActorId, OrganizationID: result.OrganizationId, CredentialID: result.CredentialId}, nil
}
func validAgentIdentityComponent(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

package client

import (
	"context"
	"errors"
	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"net/http"
	"net/url"
	"strings"
	"time"

	hconfig "github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/protocol"
	"go.opentelemetry.io/otel/propagation"
	agentclient "media_agent/hertz_gen/media_auth/agent_token_service"
	authclient "media_agent/hertz_gen/media_auth/auth_service"
	controlclient "media_agent/hertz_gen/media_control/agent_identity_service"
	agentpb "media_agent/hertz_gen/model/media_auth/agent_token"
	authpb "media_agent/hertz_gen/model/media_auth/auth"
	controlpb "media_agent/hertz_gen/model/media_control/agent_identity"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/tracecontext"
)

var (
	ErrAgentTokenUnavailable  = errors.New("Agent Token service unavailable")
	ErrAgentTokenUnauthorized = errors.New("User identity required")
	ErrAgentTokenInvalid      = errors.New("invalid Agent Token request")
	ErrAgentTokenForbidden    = errors.New("Agent delegation forbidden")
	ErrAgentTokenNotFound     = errors.New("Agent configuration not found")
	ErrAgentTokenConflict     = errors.New("Agent Token command already processed")
)

type GatewayUser struct{ UserID, OrganizationID, Role string }
type AgentIdentity struct {
	ConfigurationID, ActorID, OwnerUserID, OrganizationID, WorkspaceID, TeamID, State string
	OwnerAuthorized                                                                   bool
}
type AgentTokenCommand struct{ UserID, ConfigurationID, Name, ExpiresAt, IdempotencyKey string }
type AgentTokenMetadata struct {
	ID             string   `json:"id"`
	SubjectUserID  string   `json:"subject_user_id"`
	ActorType      string   `json:"actor_type"`
	ActorID        string   `json:"actor_id"`
	OrganizationID string   `json:"organization_id"`
	Name           string   `json:"name"`
	Scopes         []string `json:"scopes"`
	Status         string   `json:"status"`
	ExpiresAt      string   `json:"expires_at"`
	CreatedBy      string   `json:"created_by"`
	CreatedAt      string   `json:"created_at"`
}
type AgentTokenResult struct {
	Token  AgentTokenMetadata `json:"api_token"`
	Secret string             `json:"secret"`
}

type AgentTokenClient struct {
	auth       authclient.Client
	issuer     agentclient.Client
	control    controlclient.Client
	verifier   *internaljwt.Verifier
	credential string
}

func NewAgentTokenClient(authURL, controlURL, controlCredential, issuanceCredential string, verifier *internaljwt.Verifier) (*AgentTokenClient, error) {
	for _, address := range []string{authURL, controlURL} {
		u, err := url.Parse(address)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, ErrAgentTokenUnavailable
		}
	}
	if controlCredential == "" || issuanceCredential == "" || verifier == nil {
		return nil, ErrAgentTokenUnavailable
	}
	decider := func(int, *protocol.Response) bool { return false }
	a, err := authclient.NewAuthServiceClient(strings.TrimRight(authURL, "/"), authclient.WithResponseResultDecider(decider), authclient.WithHertzClientMiddleware(agentTokenHeaders()))
	if err != nil {
		return nil, ErrAgentTokenUnavailable
	}
	i, err := agentclient.NewAgentTokenServiceClient(strings.TrimRight(authURL, "/"), agentclient.WithResponseResultDecider(decider), agentclient.WithHertzClientMiddleware(agentTokenHeaders()))
	if err != nil {
		return nil, ErrAgentTokenUnavailable
	}
	c, err := controlclient.NewAgentIdentityServiceClient(strings.TrimRight(controlURL, "/"), controlclient.WithResponseResultDecider(decider), controlclient.WithHertzClientMiddleware(agentTokenHeaders()), controlclient.WithHeader(http.Header{"X-Control-Service-Token": {controlCredential}, "X-Control-Acting-User-ID": {"media-agent-gateway"}, "X-Control-Acting-Role": {"system"}}))
	if err != nil {
		return nil, ErrAgentTokenUnavailable
	}
	return &AgentTokenClient{auth: a, issuer: i, control: c, verifier: verifier, credential: issuanceCredential}, nil
}

func traceHeaders(ctx context.Context) http.Header {
	headers := http.Header{}
	carrier := propagation.MapCarrier{}
	tracecontext.Inject(ctx, carrier)
	for key, value := range carrier {
		headers.Set(key, value)
	}
	return headers
}

type agentTokenHeadersKey struct{}

func agentTokenHeaders() hclient.Middleware {
	return func(next hclient.Endpoint) hclient.Endpoint {
		return func(ctx context.Context, req *protocol.Request, resp *protocol.Response) error {
			headers, _ := ctx.Value(agentTokenHeadersKey{}).(http.Header)
			for key, values := range headers {
				for _, value := range values {
					req.Header.Set(key, value)
				}
			}
			return next(ctx, req, resp)
		}
	}
}

// Authenticate accepts a signed Edge assertion only, never arbitrary acting-user
// headers or a raw API Token. Auth resolves browser subjects and current User.
func (c *AgentTokenClient) Authenticate(ctx context.Context, authorization, cookie string) (GatewayUser, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if !strings.HasPrefix(authorization, "Bearer ") {
		return GatewayUser{}, ErrAgentTokenUnauthorized
	}
	claims, err := c.verifier.Verify(ctx, strings.TrimPrefix(authorization, "Bearer "))
	if err != nil || ctx.Err() != nil {
		return GatewayUser{}, ErrAgentTokenUnauthorized
	}
	var principal GatewayUser
	if claims.AuthSource == "api_token" {
		user, ok := strings.CutPrefix(claims.Subject, "User:")
		if !ok || user == "" || claims.CredentialID == "" || (claims.Actor != nil && claims.Actor.Sub != "" && claims.Actor.Sub != claims.Subject) || !hasScope(claims.Scope, "api.access") {
			return GatewayUser{}, ErrAgentTokenUnauthorized
		}
		principal = GatewayUser{UserID: user, OrganizationID: claims.OrganizationID}
	} else if claims.Actor != nil && claims.Actor.Sub != "" {
		return GatewayUser{}, ErrAgentTokenUnauthorized
	}
	headers := traceHeaders(ctx)
	headers.Set("Authorization", authorization)
	headers.Set("Cookie", cookie)
	value, response, err := c.auth.GetMe(context.WithValue(ctx, agentTokenHeadersKey{}, headers), &authpb.GetMeRequest{}, hconfig.WithRequestTimeout(5*time.Second))
	if err != nil || ctx.Err() != nil || response == nil {
		return GatewayUser{}, ErrAgentTokenUnavailable
	}
	if response.StatusCode() == 401 || response.StatusCode() == 403 {
		return GatewayUser{}, ErrAgentTokenUnauthorized
	}
	if response.StatusCode() != 200 || value == nil || value.GetId() == "" {
		return GatewayUser{}, ErrAgentTokenUnavailable
	}
	if principal.UserID != "" && principal.UserID != value.GetId() {
		return GatewayUser{}, ErrAgentTokenUnauthorized
	}
	principal.UserID = value.GetId()
	principal.Role = value.GetRole()
	return principal, nil
}

func hasScope(scopes, required string) bool {
	for _, scope := range strings.Fields(scopes) {
		if scope == required {
			return true
		}
	}
	return false
}

func (c *AgentTokenClient) ResolveAgentIdentity(ctx context.Context, id string) (AgentIdentity, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	value, response, err := c.control.ResolveAgentIdentity(context.WithValue(ctx, agentTokenHeadersKey{}, traceHeaders(ctx)), &controlpb.ResolveAgentIdentityRequest{AgentConfigurationId: id}, hconfig.WithRequestTimeout(5*time.Second))
	if err != nil || ctx.Err() != nil || response == nil {
		return AgentIdentity{}, ErrAgentTokenUnavailable
	}
	if response.StatusCode() == 404 {
		return AgentIdentity{}, ErrAgentTokenNotFound
	}
	if response.StatusCode() != 200 || value == nil {
		return AgentIdentity{}, ErrAgentTokenUnavailable
	}
	return AgentIdentity{ConfigurationID: value.GetAgentConfigurationId(), ActorID: value.GetActorId(), OwnerUserID: value.GetOwnerUserId(), OrganizationID: value.GetOrganizationId(), WorkspaceID: value.GetWorkspaceId(), TeamID: value.GetTeamId(), State: value.GetWorkspaceState(), OwnerAuthorized: value.GetOwnerAuthorized()}, nil
}

// Issue is intentionally a single attempt. Generated upstream errors are never
// returned because their bodies can contain Secret or service credentials.
func (c *AgentTokenClient) Issue(ctx context.Context, cmd AgentTokenCommand) (AgentTokenResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	headers := traceHeaders(ctx)
	headers.Set("X-Agent-Gateway-Internal-Token", c.credential)
	headers.Set("X-Media-Auth-Acting-User-ID", cmd.UserID)
	value, response, err := c.issuer.CreateActorAPIToken(context.WithValue(ctx, agentTokenHeadersKey{}, headers), &agentpb.CreateAgentTokenRequest{AgentConfigurationId: cmd.ConfigurationID, Name: cmd.Name, ExpiresAt: cmd.ExpiresAt, IdempotencyKey: cmd.IdempotencyKey, Scopes: []string{"agent.gateway"}}, hconfig.WithRequestTimeout(15*time.Second))
	if err != nil || ctx.Err() != nil || response == nil {
		return AgentTokenResult{}, ErrAgentTokenUnavailable
	}
	switch response.StatusCode() {
	case 400:
		return AgentTokenResult{}, ErrAgentTokenInvalid
	case 403:
		return AgentTokenResult{}, ErrAgentTokenForbidden
	case 404:
		return AgentTokenResult{}, ErrAgentTokenNotFound
	case 409:
		return AgentTokenResult{}, ErrAgentTokenConflict
	}
	if response.StatusCode() != 201 || value == nil || value.GetApiToken() == nil || value.GetSecret() == "" {
		return AgentTokenResult{}, ErrAgentTokenUnavailable
	}
	t := value.GetApiToken()
	if t.GetId() == "" || t.GetStatus() != "active" || t.GetOrganizationId() == "" || t.GetActorType() != "agent" || t.GetActorId() != "Agent:"+cmd.ConfigurationID || t.GetSubjectUserId() != cmd.UserID || len(t.GetScopes()) != 1 || t.GetScopes()[0] != "agent.gateway" {
		return AgentTokenResult{}, ErrAgentTokenUnavailable
	}
	return AgentTokenResult{Token: AgentTokenMetadata{ID: t.GetId(), SubjectUserID: t.GetSubjectUserId(), ActorType: t.GetActorType(), ActorID: t.GetActorId(), OrganizationID: t.GetOrganizationId(), Name: t.GetName(), Scopes: t.GetScopes(), Status: t.GetStatus(), ExpiresAt: t.GetExpiresAt(), CreatedBy: t.GetCreatedBy(), CreatedAt: t.GetCreatedAt()}, Secret: value.GetSecret()}, nil
}

// Package control wraps the credential-free Control identity contract. Generated
// HTTP DTOs and internal service credentials never reach the token domain.
package control

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	hconfig "github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/protocol"
	"go.opentelemetry.io/otel/propagation"
	"media_agent/hertz_gen/media_control/agent_identity_service"
	pb "media_agent/hertz_gen/model/media_control/agent_identity"
	"media_agent/hertz_infra/tracecontext"
)

var ErrUnavailable = errors.New("Control agent identity unavailable")
var ErrNotFound = errors.New("Control agent identity not found")
var ErrInvalid = errors.New("invalid Control agent identity configuration")

type AgentIdentity struct {
	ConfigurationID, ActorID, WorkspaceID, OwnerUserID, OrganizationID, TeamID, WorkspaceState string
	OwnerAuthorized                                                                            bool
}

type Client struct {
	client  agent_identity_service.Client
	timeout time.Duration
}

func New(baseURL, internalToken string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || internalToken == "" {
		return nil, ErrInvalid
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client, err := agent_identity_service.NewAgentIdentityServiceClient(strings.TrimRight(baseURL, "/"),
		agent_identity_service.WithHeader(http.Header{
			"X-Control-Service-Token": []string{internalToken}, "X-Control-Acting-User-Id": []string{"media-auth"}, "X-Control-Acting-Role": []string{"system"},
		}), agent_identity_service.WithHertzClientMiddleware(propagateIdentityTrace()),
		agent_identity_service.WithResponseResultDecider(func(int, *protocol.Response) bool { return false }))
	if err != nil {
		return nil, ErrInvalid
	}
	return &Client{client: client, timeout: timeout}, nil
}

func (c *Client) ResolveAgentIdentity(ctx context.Context, id string) (AgentIdentity, error) {
	if c == nil || c.client == nil || !validID(id) || ctx.Err() != nil {
		return AgentIdentity{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return AgentIdentity{}, ErrUnavailable
	}
	result, response, err := c.client.ResolveAgentIdentity(ctx, &pb.ResolveAgentIdentityRequest{AgentConfigurationId: id}, hconfig.WithRequestTimeout(remaining))
	// Generated client errors may contain upstream bodies. Never return them.
	if err == nil && ctx.Err() == nil && response != nil && response.StatusCode() == http.StatusNotFound {
		return AgentIdentity{}, ErrNotFound
	}
	if err != nil || ctx.Err() != nil || response == nil || response.StatusCode() != http.StatusOK || result == nil {
		return AgentIdentity{}, ErrUnavailable
	}
	return AgentIdentity{ConfigurationID: result.GetAgentConfigurationId(), ActorID: result.GetActorId(), WorkspaceID: result.GetWorkspaceId(), OwnerUserID: result.GetOwnerUserId(), OrganizationID: result.GetOrganizationId(), TeamID: result.GetTeamId(), WorkspaceState: result.GetWorkspaceState(), OwnerAuthorized: result.GetOwnerAuthorized()}, nil
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func propagateIdentityTrace() hclient.Middleware {
	return func(next hclient.Endpoint) hclient.Endpoint {
		return func(ctx context.Context, req *protocol.Request, resp *protocol.Response) error {
			carrier := propagation.MapCarrier{}
			tracecontext.Inject(ctx, carrier)
			for key, value := range carrier {
				req.Header.Set(key, value)
			}
			return next(ctx, req, resp)
		}
	}
}

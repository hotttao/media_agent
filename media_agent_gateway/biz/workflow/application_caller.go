package workflow

import (
	"context"

	"media_agent/hertz_infra/authn/applicationcaller"
)

type ApplicationCallerAuthority interface {
	Resolve(context.Context, string, string, MCPContext) (applicationcaller.Identity, bool, error)
}
type ApplicationCaller struct {
	Authority ApplicationCallerAuthority
	Signer    *applicationcaller.Signer
}

func (w *MCPGateway) WithApplicationCaller(c *ApplicationCaller) *MCPGateway {
	w.applicationCaller = c
	return w
}
func (c *ApplicationCaller) Headers(scope MCPContext, definition, endpoint string) func(context.Context) (map[string]string, error) {
	return func(ctx context.Context) (map[string]string, error) {
		if scope.Kind != "managed_agent" || scope.SessionID == "" || scope.TeamID == "" {
			return nil, ErrMCPForbidden
		}
		p, enabled, e := c.Authority.Resolve(ctx, definition, endpoint, scope)
		if e != nil {
			return nil, e
		}
		if !enabled {
			return nil, nil
		}
		token, e := c.Signer.Sign(p, scope.ExpiresAt)
		if e != nil {
			return nil, e
		}
		return map[string]string{applicationcaller.Header: token}, nil
	}
}

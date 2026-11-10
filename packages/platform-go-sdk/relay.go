package platformsdk

import (
	"context"
	handler "media_agent/packages/platform-go-sdk/biz/handler/session_relay"
	flow "media_agent/packages/platform-go-sdk/biz/workflow/session_relay"
	"net/http"
)

type RelayConfig struct {
	Prefix, Origin string
	Authenticate   func(*http.Request) (context.Context, error)
	// Resolve maps an application-owned key to a user client and a trusted workspace ID.
	Resolve func(context.Context, string) (*UserClient, string, error)
}

func NewRelay(c RelayConfig) (http.Handler, error) {
	var resolve func(context.Context, string) (*flow.Flow, string, error)
	if c.Resolve != nil {
		resolve = func(ctx context.Context, key string) (*flow.Flow, string, error) {
			u, id, err := c.Resolve(ctx, key)
			if err != nil || u == nil {
				return nil, "", err
			}
			return u.SessionAccess, id, nil
		}
	}
	return handler.New(handler.Config{Prefix: c.Prefix, Origin: c.Origin, Authenticate: c.Authenticate, Resolve: resolve})
}

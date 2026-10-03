package middleware

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	client "media_agent/media_agent_gateway/biz/shared/client"
)

type AgentTokenAuthenticator interface {
	Authenticate(context.Context, string, string) (client.GatewayUser, error)
}
type managementUserKey struct{}

var managementAuthenticator AgentTokenAuthenticator

func ConfigureAgentTokenManagement(value AgentTokenAuthenticator) { managementAuthenticator = value }
func ManagementUser(ctx context.Context) client.GatewayUser {
	value, _ := ctx.Value(managementUserKey{}).(client.GatewayUser)
	return value
}
func AgentTokenManagement() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if managementAuthenticator == nil {
			c.JSON(503, map[string]string{"error": "Agent Token management unavailable"})
			c.Abort()
			return
		}
		user, err := managementAuthenticator.Authenticate(ctx, string(c.GetHeader("Authorization")), string(c.GetHeader("Cookie")))
		if err != nil || user.UserID == "" {
			status := 503
			if errors.Is(err, client.ErrAgentTokenUnauthorized) {
				status = 401
			}
			c.JSON(status, map[string]string{"error": "Agent Token management identity unavailable"})
			c.Abort()
			return
		}
		c.Next(context.WithValue(ctx, managementUserKey{}, user))
	}
}

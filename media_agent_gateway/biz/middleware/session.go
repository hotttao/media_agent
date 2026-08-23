package middleware

import (
	"context"
	"crypto/subtle"
	"fmt"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/golang-jwt/jwt/v5"
)

type sessionKey struct{}
type Claims struct {
	Sub string `json:"sub"`
	jwt.RegisteredClaims
}

var secret []byte
var internalToken string

func Configure(jwtSecret, serviceToken string) {
	secret, internalToken = []byte(jwtSecret), serviceToken
}

func UserID(ctx context.Context) string { value, _ := ctx.Value(sessionKey{}).(string); return value }

func Session() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		token := string(c.Cookie("token"))
		claims := &Claims{}
		parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected JWT algorithm")
			}
			return secret, nil
		})
		if err != nil || !parsed.Valid || claims.Sub == "" {
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid session"})
			c.Abort()
			return
		}
		c.Next(context.WithValue(ctx, sessionKey{}, claims.Sub))
	}
}

func InternalService() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !matchesInternalToken(string(c.GetHeader("X-Control-Service-Token")), internalToken) {
			c.JSON(consts.StatusForbidden, map[string]string{"error": "internal service identity is required"})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}

func matchesInternalToken(provided, configured string) bool {
	return provided != "" && configured != "" && len(provided) == len(configured) && subtle.ConstantTimeCompare([]byte(provided), []byte(configured)) == 1
}

package middleware

import (
	"context"
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

func Configure(jwtSecret string)        { secret = []byte(jwtSecret) }
func UserID(ctx context.Context) string { value, _ := ctx.Value(sessionKey{}).(string); return value }
func Session() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		claims := &Claims{}
		parsed, err := jwt.ParseWithClaims(string(c.Cookie("token")), claims, func(t *jwt.Token) (any, error) {
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

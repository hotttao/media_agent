package middleware

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"

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
var serviceToken string

func Configure(jwtSecret, internalToken string) {
	secret = []byte(jwtSecret)
	serviceToken = internalToken
}
func UserID(ctx context.Context) string { value, _ := ctx.Value(sessionKey{}).(string); return value }
func Session() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if userID, ok := internalUser(string(c.GetHeader("X-Control-Service-Token")), serviceToken, string(c.GetHeader("X-Control-Acting-User-ID"))); ok {
			c.Next(context.WithValue(ctx, sessionKey{}, userID))
			return
		}
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

func internalUser(provided, configured, actingUserID string) (string, bool) {
	actingUserID = strings.TrimSpace(actingUserID)
	if provided == "" || configured == "" || actingUserID == "" || len(provided) != len(configured) {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(configured)) != 1 {
		return "", false
	}
	return actingUserID, true
}

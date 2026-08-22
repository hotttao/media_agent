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
	Sub  string `json:"sub"`
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type Principal struct {
	UserID, Role string
	Internal     bool
}

var secret []byte
var serviceToken string

func Configure(jwtSecret, internalToken string) {
	secret = []byte(jwtSecret)
	serviceToken = internalToken
}
func principal(ctx context.Context) Principal {
	value, _ := ctx.Value(sessionKey{}).(Principal)
	return value
}
func UserID(ctx context.Context) string   { return principal(ctx).UserID }
func Role(ctx context.Context) string     { return principal(ctx).Role }
func IsInternal(ctx context.Context) bool { return principal(ctx).Internal }
func Session() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if userID, ok := internalUser(string(c.GetHeader("X-Control-Service-Token")), serviceToken, string(c.GetHeader("X-Control-Acting-User-ID"))); ok {
			role := normalizeRole(string(c.GetHeader("X-Control-Acting-Role")))
			c.Next(context.WithValue(ctx, sessionKey{}, Principal{UserID: userID, Role: role, Internal: true}))
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
		c.Next(context.WithValue(ctx, sessionKey{}, Principal{UserID: claims.Sub, Role: normalizeRole(claims.Role)}))
	}
}

func RequireInternalSystem() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !IsInternal(ctx) || Role(ctx) != "system" {
			c.JSON(consts.StatusForbidden, map[string]string{"error": "internal system identity is required"})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}

func RequireAdmin() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		role := Role(ctx)
		if role != "admin" && role != "system" {
			c.JSON(consts.StatusForbidden, map[string]string{"error": "platform administrator role is required"})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}

func normalizeRole(role string) string {
	switch strings.TrimSpace(role) {
	case "admin":
		return "admin"
	case "system":
		return "system"
	default:
		return "user"
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

package middleware

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/golang-jwt/jwt/v5"
	"media_agent/hertz_infra/authn/internaljwt"
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
var internalVerifier *internaljwt.Verifier
var kratosPublicURL, mediaAuthURL, mediaAuthInternalToken string
var httpClient = &http.Client{Timeout: 5 * time.Second}

func Configure(jwtSecret, internalToken string) {
	secret = []byte(jwtSecret)
	serviceToken = internalToken
}

func ConfigureOry(verifier *internaljwt.Verifier, kratosURL, authURL, authInternalToken string) {
	internalVerifier = verifier
	kratosPublicURL = strings.TrimRight(kratosURL, "/")
	mediaAuthURL = strings.TrimRight(authURL, "/")
	mediaAuthInternalToken = authInternalToken
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
		if internalVerifier != nil {
			authHeader := string(c.GetHeader("Authorization"))
			if strings.HasPrefix(authHeader, "Bearer ") {
				claims, err := internalVerifier.Verify(ctx, strings.TrimPrefix(authHeader, "Bearer "))
				if err == nil {
					if claims.AuthSource == "api_token" {
						userID, ok := strings.CutPrefix(claims.Subject, "User:")
						if !ok || userID == "" || (claims.Actor != nil && claims.Actor.Sub != "" && claims.Actor.Sub != claims.Subject) {
							c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid API Token principal"})
							c.Abort()
							return
						}
						c.Next(context.WithValue(ctx, sessionKey{}, Principal{UserID: userID, Role: normalizeRole(claims.Role)}))
						return
					}
					resolved, resolveErr := resolveKratosPrincipal(ctx, string(c.GetHeader("Cookie")), claims.Subject)
					if resolveErr != nil {
						c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid internal identity"})
						c.Abort()
						return
					}
					c.Next(context.WithValue(ctx, sessionKey{}, resolved))
					return
				}
			}
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

func resolveKratosPrincipal(ctx context.Context, cookie, expectedIdentityID string) (Principal, error) {
	if kratosPublicURL == "" || mediaAuthURL == "" || mediaAuthInternalToken == "" || cookie == "" {
		return Principal{}, fmt.Errorf("Ory identity bridge is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kratosPublicURL+"/sessions/whoami", nil)
	if err != nil {
		return Principal{}, err
	}
	req.Header.Set("Cookie", cookie)
	resp, err := httpClient.Do(req)
	if err != nil {
		return Principal{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Principal{}, fmt.Errorf("Kratos whoami status %d", resp.StatusCode)
	}
	var session struct {
		Identity struct {
			ID     string `json:"id"`
			Traits struct {
				Email string                       `json:"email"`
				Name  struct{ First, Last string } `json:"name"`
			} `json:"traits"`
		} `json:"identity"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return Principal{}, err
	}
	if session.Identity.ID == "" || session.Identity.ID != expectedIdentityID {
		return Principal{}, fmt.Errorf("Kratos identity does not match assertion")
	}
	name := strings.TrimSpace(session.Identity.Traits.Name.First + " " + session.Identity.Traits.Name.Last)
	body, err := json.Marshal(map[string]string{"identity_id": session.Identity.ID, "email": session.Identity.Traits.Email, "name": name})
	if err != nil {
		return Principal{}, err
	}
	resolveReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mediaAuthURL+"/internal/identities/kratos:resolve", bytes.NewReader(body))
	if err != nil {
		return Principal{}, err
	}
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveReq.Header.Set("X-Media-Auth-Internal-Token", mediaAuthInternalToken)
	resolveResp, err := httpClient.Do(resolveReq)
	if err != nil {
		return Principal{}, err
	}
	defer resolveResp.Body.Close()
	if resolveResp.StatusCode != http.StatusOK {
		return Principal{}, fmt.Errorf("resolve Kratos identity status %d", resolveResp.StatusCode)
	}
	var user struct {
		ID   string `json:"id"`
		Sub  string `json:"sub"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(resolveResp.Body).Decode(&user); err != nil {
		return Principal{}, err
	}
	if user.Sub == "" {
		user.Sub = user.ID
	}
	if user.Sub == "" {
		return Principal{}, fmt.Errorf("resolved user id is empty")
	}
	return Principal{UserID: user.Sub, Role: normalizeRole(user.Role)}, nil
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

func RequireInternal() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !isInternalPrincipal(principal(ctx)) {
			c.JSON(consts.StatusForbidden, map[string]string{"error": "internal service identity is required"})
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

func isInternalPrincipal(value Principal) bool {
	return value.Internal && strings.TrimSpace(value.UserID) != ""
}

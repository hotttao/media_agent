package middleware

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	configpb "media_agent/hertz_gen/config"
	"media_agent/media_auth/biz/domain/auth"
	tokenservice "media_agent/media_auth/biz/service/token"
	"media_agent/media_auth/biz/shared/session"
)

type sessionUserReader struct {
	auth.UserRepo
	reads int
}

func (r *sessionUserReader) FindByID(context.Context, string) (auth.User, error) {
	r.reads++
	return auth.User{ID: "owner", Status: "active", AuthVersion: 2}, nil
}

func TestUntrustedBearerCannotUseCookieOrReadCredentials(t *testing.T) {
	signer, err := session.NewSigner(&configpb.AuthConfig{JwtSecret: "session-test"})
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := signer.Sign(auth.Session{Sub: "owner", AuthVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	reader := &sessionUserReader{}
	h := server.New()
	h.Use(SessionWithToken(signer, tokenservice.NewService(reader)))
	h.GET("/api/protected", func(ctx context.Context, c *app.RequestContext) {
		if SessionFromContext(ctx) == nil {
			t.Error("Cookie session missing")
		}
		c.Status(200)
	})
	if result := ut.PerformRequest(h.Engine, "GET", "/api/protected", nil, ut.Header{Key: "Cookie", Value: "token=" + cookie}); result.Code != 200 || reader.reads != 1 {
		t.Fatal("independent Cookie session validation broken")
	}
	for _, raw := range []string{"Bearer tos_legacy", "Bearer unknown-api-token", "Bearer invalid.jwt.value", "bearer tos_legacy", "BEARER tos_legacy", "Basic untrusted"} {
		result := ut.PerformRequest(h.Engine, "GET", "/api/protected", nil, ut.Header{Key: "Cookie", Value: "token=" + cookie}, ut.Header{Key: "Authorization", Value: raw})
		if result.Code != 401 || reader.reads != 1 {
			t.Fatal("untrusted Bearer downgraded to Cookie/current User lookup")
		}
	}
}

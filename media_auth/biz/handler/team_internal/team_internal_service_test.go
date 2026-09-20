package team_internal

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestAuthorizeInternalFailsClosed(t *testing.T) {
	t.Setenv("MEDIA_AUTH_INTERNAL_TOKEN", "internal-secret")

	withoutToken := app.NewContext(0)
	if authorizeInternal(withoutToken) {
		t.Fatal("missing internal token must be rejected")
	}
	if got := withoutToken.Response.StatusCode(); got != 401 {
		t.Fatalf("missing token status=%d, want 401", got)
	}

	wrongToken := app.NewContext(0)
	wrongToken.Request.Header.Set("X-Media-Auth-Internal-Token", "wrong")
	if authorizeInternal(wrongToken) {
		t.Fatal("wrong internal token must be rejected")
	}

	validToken := app.NewContext(0)
	validToken.Request.Header.Set("X-Media-Auth-Internal-Token", "internal-secret")
	if !authorizeInternal(validToken) {
		t.Fatal("matching internal token must be accepted")
	}
}

func TestAuthorizeInternalRejectsUnsetConfiguration(t *testing.T) {
	t.Setenv("MEDIA_AUTH_INTERNAL_TOKEN", "")
	c := app.NewContext(0)
	if authorizeInternal(c) {
		t.Fatal("unset internal token configuration must fail closed")
	}
}

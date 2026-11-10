package appcredential

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOnlineVerificationRejectsFallbackAndInvalidClaims(t *testing.T) {
	mode := "valid"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/internal/apps/verify" || r.Header.Get("X-Media-Auth-Internal-Token") != "secret" {
			t.Error("wrong private protocol")
		}
		if mode == "redirect" {
			http.Redirect(w, r, "https://untrusted.invalid", 302)
			return
		}
		if mode == "denied" {
			w.WriteHeader(403)
			return
		}
		if mode == "unavailable" {
			w.WriteHeader(500)
			return
		}
		p := Principal{ApplicationID: "app", UserID: "user", ExternalID: "external", TeamID: "team", OrganizationID: "org", Scopes: []string{"workspace.read"}, ExpiresAt: time.Now().Add(time.Minute).Unix()}
		if mode == "expired" {
			p.ExpiresAt = 1
		}
		if mode == "incomplete" {
			p.TeamID = ""
		}
		_ = json.NewEncoder(w).Encode(p)
	}))
	defer server.Close()
	c := New(server.URL, "secret")
	token := "mac_" + strings.Repeat("a", 64)
	ctx := context.Background()
	if _, e := c.Verify(ctx, token, "workspace.read"); e != nil {
		t.Fatal(e)
	}
	for _, m := range []string{"expired", "incomplete", "denied", "redirect", "unavailable"} {
		mode = m
		if _, e := c.Verify(ctx, token, "workspace.read"); e == nil {
			t.Fatal("accepted " + m)
		}
	}
	mode = "valid"
	if _, e := c.Verify(ctx, token, "session.chat"); !errors.Is(e, ErrDenied) {
		t.Fatal("scope escalation")
	}
	before := calls
	if _, e := New(server.URL, "").Verify(ctx, token, "workspace.read"); !errors.Is(e, ErrUnavailable) || calls != before {
		t.Fatal("empty service credential made request")
	}
	if _, e := c.Verify(ctx, "cookie-value", "workspace.read"); !errors.Is(e, ErrDenied) || calls != before {
		t.Fatal("invalid bearer attempted fallback")
	}
}

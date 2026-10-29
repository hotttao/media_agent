package workflow

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/hertz_infra/authn/resourceaccess"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMCPAdmissionRequiresServiceAndCurrentModuleAuthority(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := mcpdelegation.NewSigner(key, "gateway", nil)
	if err != nil {
		t.Fatal(err)
	}
	var allowed, outage atomic.Bool
	allowed.Store(true)
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jwks" {
			json.NewEncoder(w).Encode(signer.PublicJWKS())
			return
		}
		reads.Add(1)
		if r.Header.Get("X-Media-Auth-Internal-Token") != "auth-secret" {
			t.Error("missing fact service authentication")
		}
		if outage.Load() {
			w.WriteHeader(503)
			return
		}
		switch r.URL.Path {
		case "/internal/organizations:byUser":
			json.NewEncoder(w).Encode(map[string]any{"organization_ids": []string{"org"}})
		case "/internal/teams:byUser":
			json.NewEncoder(w).Encode(map[string]any{"team_ids": []string{"team"}})
		case "/internal/team-authorization/check":
			var request map[string]string
			json.NewDecoder(r.Body).Decode(&request)
			yes := allowed.Load() || request["permission"] == "view"
			json.NewEncoder(w).Encode(map[string]any{"team_id": "team", "organization_id": "org", "team_role": "member", "organization_role": "member", "team_active": true, "organization_active": true, "allowed": yes})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	secret := strings.Repeat("s", 32)
	admission := MCPAdmission{Verifier: internaljwt.NewVerifier(server.URL+"/jwks", "gateway", mcpdelegation.Materials, nil), ServiceToken: secret, Facts: resourceaccess.New(server.URL, "auth-secret", nil)}
	grant := mcpdelegation.Grant{Principal: mcpdelegation.Principal{Kind: "user", UserID: "u", ActorID: "User:u", CredentialID: "c", OrganizationID: "org"}, Audience: mcpdelegation.Materials, CallID: "call", SourceID: "media-workflow", Operation: "jobs_get", Presenter: mcpdelegation.Backend, ExpiresAt: time.Now().Add(time.Minute)}
	raw, err := signer.Sign(grant)
	if err != nil {
		t.Fatal(err)
	}
	authorization := "Bearer " + raw
	const path = "/api/v1/generated-assets/a"
	if _, err = admission.Authenticate(context.Background(), authorization, "wrong", "GET", path); err == nil || reads.Load() != 0 {
		t.Fatal("invalid presenter reached facts")
	}
	access, err := admission.Authenticate(context.Background(), authorization, secret, "GET", path)
	if err != nil || access.UserID != "u" || len(access.TeamIDs) != 1 {
		t.Fatalf("valid identity failed: %+v %v", access, err)
	}
	allowed.Store(false)
	if _, err = admission.Authenticate(context.Background(), authorization, secret, "GET", path); !errors.Is(err, mcpdelegation.ErrInvalid) {
		t.Fatal("module revocation ignored", err)
	}
	outage.Store(true)
	if _, err = admission.Authenticate(context.Background(), authorization, secret, "GET", path); !errors.Is(err, mcpdelegation.ErrAuthorityUnavailable) {
		t.Fatal("authority outage not distinguished", err)
	}
	outage.Store(false)
	allowed.Store(true)
	if _, err = admission.Authenticate(context.Background(), authorization, secret, "POST", "/api/v1/jobs"); err == nil {
		t.Fatal("read delegation authorized write")
	}
	grant.Audience = mcpdelegation.Backend
	grant.Method = "tools/list"
	grant.Presenter = ""
	grant.Operation = ""
	backend, err := signer.Sign(grant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admission.Authenticate(context.Background(), "Bearer "+backend, secret, "GET", path); err == nil {
		t.Fatal("backend audience authorized business API")
	}
}

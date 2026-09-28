package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"media_agent/hertz_infra/authn/internaljwt"
)

func TestVerifyWorkerIdentityAcceptsTokenOnlyPrincipal(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{jwkForTest(&key.PublicKey)}})
	}))
	defer server.Close()
	issuer := "http://oathkeeper.test/"
	claims := jwt.MapClaims{
		"iss": issuer, "aud": []string{"media-workflow"}, "sub": "User:user-1", "act": map[string]any{"sub": "Worker:worker-1"}, "principal_type": "user",
		"organization_id": "org-1", "credential_id": "credential-1", "auth_source": "api_token",
		"scope": "workflow.worker.poll workflow.worker.complete", "username": "svc:desktop", "name": "Desktop", "role": "user",
		"iat": time.Now().Add(-time.Second).Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	}
	raw := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	raw.Header["kid"] = "test-key"
	signed, err := raw.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	middleware := New(&Config{WorkerJWTVerifier: internaljwt.NewVerifier(server.URL, issuer, "media-workflow", server.Client())})

	principal, user, err := middleware.verifyWorkerIdentity(context.Background(), signed)
	if err != nil {
		t.Fatalf("verifyWorkerIdentity() error = %v", err)
	}
	if principal.UserID != "user-1" || principal.ActorID != "Worker:worker-1" || principal.OrganizationID != "org-1" || principal.CredentialID != "credential-1" || principal.AuthSource != "api_token" || !principal.HasScope("workflow.worker.complete") {
		t.Fatalf("principal = %#v", principal)
	}
	if user.Username != "svc:desktop" || user.Name != "Desktop" {
		t.Fatalf("user = %#v", user)
	}
}

func TestVerifyWorkerIdentityRejectsMissingWorkerVerifier(t *testing.T) {
	middleware := New(&Config{})
	if _, _, err := middleware.verifyWorkerIdentity(context.Background(), "not-a-token"); err == nil {
		t.Fatal("verifyWorkerIdentity() should reject when the worker verifier is not configured")
	}
}

func jwkForTest(key *rsa.PublicKey) map[string]string {
	exponent := make([]byte, 4)
	binary.BigEndian.PutUint32(exponent, uint32(key.E))
	for len(exponent) > 1 && exponent[0] == 0 {
		exponent = exponent[1:]
	}
	return map[string]string{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "test-key",
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(exponent),
	}
}

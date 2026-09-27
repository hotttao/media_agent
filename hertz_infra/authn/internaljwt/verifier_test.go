package internaljwt

import (
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
)

func TestVerifierAcceptsExpectedIssuerAudienceAndSubject(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var exponent [4]byte
		binary.BigEndian.PutUint32(exponent[:], uint32(key.PublicKey.E))
		first := 0
		for first < 3 && exponent[first] == 0 {
			first++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(exponent[first:]),
		}}})
	}))
	defer server.Close()

	now := time.Now()
	claims := Claims{PrincipalType: "user", OrganizationID: "org-1", CredentialID: "token-1", Scope: "workflow.worker.poll workflow.worker.complete", AuthSource: "service_token", RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "http://oathkeeper:4456/", Subject: "identity-1",
		Audience: jwt.ClaimStrings{"internal-api"}, IssuedAt: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewVerifier(server.URL, "http://oathkeeper:4456/", "internal-api", server.Client()).Verify(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "identity-1" {
		t.Fatalf("subject=%q", got.Subject)
	}
	if got.OrganizationID != "org-1" || got.CredentialID != "token-1" || got.AuthSource != "service_token" {
		t.Fatalf("worker claims=%+v", got)
	}
}

package internaljwt

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestVerifierRejectsForeignIssuerWithoutJWKSAndStillChecksSignature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewAgentSigner(key, "expected", nil)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(signer.PublicJWKS())
	}))
	defer server.Close()
	v := NewVerifier(server.URL, "expected", "internal-api", server.Client())
	forge, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, issuer, audience string
		key                    *rsa.PrivateKey
		valid                  bool
		reads                  int32
	}{
		{"foreign issuer", "foreign", "internal-api", key, false, 0},
		{"missing issuer", "", "internal-api", key, false, 0},
		{"valid", "expected", "internal-api", key, true, 1},
		{"forged matching issuer", "expected", "internal-api", forge, false, 1},
		{"wrong audience", "expected", "other", key, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := Claims{PrincipalType: "user", RegisteredClaims: jwt.RegisteredClaims{Issuer: tc.issuer, Subject: "User:test", Audience: jwt.ClaimStrings{tc.audience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = signer.kid
			raw, err := token.SignedString(tc.key)
			if err != nil {
				t.Fatal(err)
			}
			_, err = v.Verify(t.Context(), raw)
			if (err == nil) != tc.valid {
				t.Fatal("incorrect authentication decision")
			}
			if requests.Load() != tc.reads {
				t.Fatalf("JWKS reads=%d want=%d", requests.Load(), tc.reads)
			}
		})
	}
}

// jwt.WithIssuer("") historically means no issuer restriction. Rejection-only
// routing must not turn an existing optional setting into a new required one.
func TestVerifierOptionalIssuerPreservesSignatureAndAudienceChecks(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	forge, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	v := NewVerifier("", "", "internal-api", nil)
	v.keys["known"] = &key.PublicKey
	for _, tc := range []struct {
		key      *rsa.PrivateKey
		audience string
		valid    bool
	}{{key, "internal-api", true}, {forge, "internal-api", false}, {key, "other", false}} {
		claims := Claims{PrincipalType: "user", RegisteredClaims: jwt.RegisteredClaims{Issuer: "gateway", Subject: "User:test", Audience: jwt.ClaimStrings{tc.audience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = "known"
		raw, err := token.SignedString(tc.key)
		if err != nil {
			t.Fatal(err)
		}
		_, err = v.Verify(t.Context(), raw)
		if (err == nil) != tc.valid {
			t.Fatal("optional issuer changed signature/audience requirements")
		}
	}
}

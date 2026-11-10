// Package caller verifies platform-signed identity assertions for business MCPs.
// Assertions establish identity, not permission to operate on business resources.
package caller

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"math/big"
	"media_agent/hertz_infra/authn/applicationcaller"
	"time"
)

var ErrInvalid = errors.New("invalid platform caller")

type Config struct {
	Issuer, Audience string
	JWKS             []byte
}
type Identity struct {
	ApplicationID, UserID, ExternalID, OrganizationID, TeamID, WorkspaceID, SessionID, AssertionID string
	ExpiresAt                                                                                      time.Time
}
type Binding struct{ WorkspaceID, SessionID string }
type Verifier struct {
	issuer, audience string
	keys             map[string]*rsa.PublicKey
}

func New(c Config) (*Verifier, error) {
	if c.Issuer == "" || c.Audience == "" || len(c.JWKS) > 1<<20 {
		return nil, ErrInvalid
	}
	var set struct {
		Keys []struct{ Kty, Use, Alg, Kid, N, E, D string }
	}
	if json.Unmarshal(c.JWKS, &set) != nil || len(set.Keys) == 0 {
		return nil, ErrInvalid
	}
	v := &Verifier{c.Issuer, c.Audience, map[string]*rsa.PublicKey{}}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Use != "sig" || k.Alg != "RS256" || k.Kid == "" || k.D != "" || v.keys[k.Kid] != nil {
			return nil, ErrInvalid
		}
		n, e := base64.RawURLEncoding.DecodeString(k.N)
		if e != nil {
			return nil, ErrInvalid
		}
		exponent, e := base64.RawURLEncoding.DecodeString(k.E)
		if e != nil || len(exponent) == 0 || len(exponent) > 4 {
			return nil, ErrInvalid
		}
		number := new(big.Int).SetBytes(n)
		exp := new(big.Int).SetBytes(exponent).Int64()
		if number.BitLen() < 2048 || number.BitLen() > 8192 || exp < 3 || exp > 2147483647 || exp%2 == 0 {
			return nil, ErrInvalid
		}
		v.keys[k.Kid] = &rsa.PublicKey{N: number, E: int(exp)}
	}
	return v, nil
}
func (v *Verifier) Verify(raw string, b Binding) (Identity, error) {
	var zero Identity
	if v == nil || len(raw) > 16384 {
		return zero, ErrInvalid
	}
	claims := &applicationcaller.Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || v.keys[kid] == nil || t.Header["typ"] != applicationcaller.Type || t.Header["crit"] != nil {
			return nil, ErrInvalid
		}
		return v.keys[kid], nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !token.Valid || claims.IssuedAt == nil || claims.NotBefore == nil || claims.ExpiresAt == nil || len(claims.Audience) != 1 {
		return zero, ErrInvalid
	}
	lifetime := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	if lifetime <= 0 || lifetime > time.Minute {
		return zero, ErrInvalid
	}
	for _, s := range []string{claims.Subject, claims.ExternalID, claims.ID, claims.OrganizationID, claims.TeamID, claims.WorkspaceID, claims.SessionID} {
		if s == "" || len(s) > 512 {
			return zero, ErrInvalid
		}
	}
	if b.WorkspaceID != "" && claims.WorkspaceID != b.WorkspaceID || b.SessionID != "" && claims.SessionID != b.SessionID {
		return zero, ErrInvalid
	}
	return Identity{ApplicationID: v.audience, UserID: claims.Subject, ExternalID: claims.ExternalID, OrganizationID: claims.OrganizationID, TeamID: claims.TeamID, WorkspaceID: claims.WorkspaceID, SessionID: claims.SessionID, AssertionID: claims.ID, ExpiresAt: claims.ExpiresAt.Time}, nil
}

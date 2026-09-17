// Package internaljwt verifies the short-lived identity assertion issued by
// Ory Oathkeeper after it has authenticated a Kratos browser session.
package internaljwt

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	PrincipalType string `json:"principal_type"`
	jwt.RegisteredClaims
}

type Verifier struct {
	url, issuer, audience string
	client                *http.Client
	mu                    sync.RWMutex
	keys                  map[string]*rsa.PublicKey
}

func NewVerifier(url, issuer, audience string, client *http.Client) *Verifier {
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &Verifier{url: url, issuer: issuer, audience: audience, client: client, keys: map[string]*rsa.PublicKey{}}
}

func (v *Verifier) Verify(ctx context.Context, raw string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("JWT header is missing kid")
		}
		if key := v.get(kid); key != nil {
			return key, nil
		}
		if err := v.refresh(ctx); err != nil {
			return nil, err
		}
		if key := v.get(kid); key != nil {
			return key, nil
		}
		return nil, fmt.Errorf("unknown kid %q", kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil {
		return nil, fmt.Errorf("verify internal JWT: %w", err)
	}
	if !token.Valid || claims.Subject == "" || claims.PrincipalType != "user" {
		return nil, errors.New("internal JWT is missing required identity claims")
	}
	return claims, nil
}

func (v *Verifier) get(kid string) *rsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.keys[kid]
}

func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch Oathkeeper JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch Oathkeeper JWKS: status %d", resp.StatusCode)
	}
	var document struct {
		Keys []struct {
			Kty, Use, Alg, Kid, N, E string
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, item := range document.Keys {
		if item.Kty != "RSA" || item.Use != "sig" || item.Alg != "RS256" || item.Kid == "" {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(item.N)
		if err != nil {
			return err
		}
		e, err := base64.RawURLEncoding.DecodeString(item.E)
		if err != nil || len(e) == 0 || len(e) > 4 {
			return errors.New("invalid RSA exponent in Oathkeeper JWKS")
		}
		var padded [4]byte
		copy(padded[4-len(e):], e)
		exponent := int(binary.BigEndian.Uint32(padded[:]))
		if exponent < 3 {
			return errors.New("invalid RSA exponent in Oathkeeper JWKS")
		}
		keys[item.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}
	}
	if len(keys) == 0 {
		return errors.New("Oathkeeper JWKS has no usable RS256 signing key")
	}
	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

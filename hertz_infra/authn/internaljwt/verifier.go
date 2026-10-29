// Package internaljwt verifies short-lived identity assertions issued by
// Ory Oathkeeper for browser sessions and service-token principals.
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
	MCPPrincipalKind   string `json:"mcp_principal_kind,omitempty"`
	MCPCallID          string `json:"mcp_call_id,omitempty"`
	MCPSourceID        string `json:"mcp_source_id,omitempty"`
	MCPOperation       string `json:"mcp_operation,omitempty"`
	MCPMethod          string `json:"mcp_method,omitempty"`
	MCPArgumentsDigest string `json:"mcp_arguments_digest,omitempty"`
	MCPPresenter       string `json:"mcp_presenter,omitempty"`

	PrincipalType      string `json:"principal_type"`
	OrganizationID     string `json:"organization_id,omitempty"`
	CredentialID       string `json:"credential_id,omitempty"`
	Scope              string `json:"scope,omitempty"`
	AuthSource         string `json:"auth_source,omitempty"`
	Username           string `json:"username,omitempty"`
	Name               string `json:"name,omitempty"`
	Role               string `json:"role,omitempty"`
	Actor              *Actor `json:"act,omitempty"`
	RequestMethod      string `json:"request_method,omitempty"`
	RequestURI         string `json:"request_uri,omitempty"`
	RequestDigest      string `json:"request_digest,omitempty"`
	RequestContentType string `json:"request_content_type,omitempty"`
	WorkspaceID        string `json:"workspace_id,omitempty"`
	jwt.RegisteredClaims
}

// Actor is the RFC 8693 actor claim. Subject identifies the delegating User;
// Actor.Sub identifies the Agent or Personal Worker acting for that User.
type Actor struct {
	Sub string `json:"sub"`
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
		// This is rejection-only routing, not authentication. Another issuer's
		// token must not trigger our JWKS endpoint; signature and all registered
		// claims are still validated below before any identity is returned.
		if v.issuer != "" && claims.Issuer != v.issuer {
			return nil, errors.New("unexpected JWT issuer")
		}
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

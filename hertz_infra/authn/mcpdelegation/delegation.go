// Package mcpdelegation carries verified identities between the MCP gateway,
// a trusted protocol adapter, and resource APIs. It owns no business ACLs.
package mcpdelegation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"media_agent/hertz_infra/authn/internaljwt"
)

const (
	Source          = "mcp_delegation"
	Backend         = "media-worker-mcp"
	Workflow        = "media-workflow-mcp"
	Materials       = "media-materials-mcp"
	MaxLifetime     = time.Minute
	WorkflowHeader  = "X-Media-MCP-Workflow-Delegation"
	MaterialsHeader = "X-Media-MCP-Materials-Delegation"
	ServiceHeader   = "X-Media-MCP-Service-Token"
)

var ErrInvalid = errors.New("invalid MCP delegation")
var ErrAuthorityUnavailable = errors.New("MCP authorization unavailable")

// Access contains live facts obtained by the receiving service, never JWT claims.
type Access struct {
	Principal
	TeamIDs []string
}

type Principal struct {
	Kind, UserID, ActorID, CredentialID, OrganizationID, WorkspaceID string
}
type Grant struct {
	Principal
	Audience, CallID, SourceID, Operation, Method, ArgumentsDigest, Presenter string
	ExpiresAt                                                                 time.Time
}
type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

type Signer struct {
	key         *rsa.PrivateKey
	issuer, kid string
	now         func() time.Time
	public      internaljwt.AgentJWKS
}

func NewSigner(key *rsa.PrivateKey, issuer string, now func() time.Time) (*Signer, error) {
	if now == nil {
		now = time.Now
	}
	existing, err := internaljwt.NewAgentSigner(key, issuer, now)
	if err != nil {
		return nil, ErrInvalid
	}
	public := existing.PublicJWKS()
	return &Signer{key, issuer, public.Keys[0].Kid, now, public}, nil
}
func LoadSigner(path, issuer string) (*Signer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrInvalid
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return nil, ErrInvalid
	}
	raw := make([]byte, info.Size())
	n, err := io.ReadFull(f, raw)
	if err != nil || n != len(raw) {
		return nil, ErrInvalid
	}
	key, err := internaljwt.ParseAgentSigningKey(raw)
	if err != nil {
		return nil, ErrInvalid
	}
	return NewSigner(key, issuer, nil)
}
func (s *Signer) PublicJWKS() internaljwt.AgentJWKS { return s.public }
func NewCallID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func ValidPrincipal(p Principal) bool {
	if !identifier(p.UserID) || !identifier(p.CredentialID) || !identifier(p.OrganizationID) || p.WorkspaceID != "" && !identifier(p.WorkspaceID) {
		return false
	}
	if p.Kind == "user" {
		return p.ActorID == "User:"+p.UserID
	}
	id, ok := strings.CutPrefix(p.ActorID, "Agent:")
	return p.Kind == "managed_agent" && ok && identifier(id) && p.WorkspaceID != ""
}
func identifier(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func validGrant(g Grant) bool {
	if !ValidPrincipal(g.Principal) || !identifier(g.CallID) || !identifier(g.SourceID) {
		return false
	}
	switch g.Audience {
	case Backend:
		if g.Presenter != "" {
			return false
		}
		switch g.Method {
		case "initialize", "ping", "tools/list", "notifications/initialized", "server/discover":
			return g.Operation == "" && g.ArgumentsDigest == ""
		case "tools/call":
			return identifier(g.Operation) && len(g.ArgumentsDigest) == 64
		default:
			return false
		}
	case Workflow, Materials:
		return g.Method == "" && g.ArgumentsDigest == "" && identifier(g.Operation) && g.Presenter == Backend
	default:
		return false
	}
}
func (s *Signer) Sign(g Grant) (string, error) {
	if s == nil || !validGrant(g) {
		return "", ErrInvalid
	}
	now := s.now().UTC().Truncate(time.Second)
	end := now.Add(MaxLifetime)
	if !g.ExpiresAt.IsZero() && g.ExpiresAt.Before(end) {
		end = g.ExpiresAt.UTC().Truncate(time.Second)
	}
	if !end.After(now) {
		return "", ErrInvalid
	}
	id, err := NewCallID()
	if err != nil {
		return "", err
	}
	c := internaljwt.Claims{PrincipalType: "user", AuthSource: Source, OrganizationID: g.OrganizationID, WorkspaceID: g.WorkspaceID, CredentialID: g.CredentialID, Actor: &internaljwt.Actor{Sub: g.ActorID}, MCPPrincipalKind: g.Kind, MCPCallID: g.CallID, MCPSourceID: g.SourceID, MCPOperation: g.Operation, MCPMethod: g.Method, MCPArgumentsDigest: g.ArgumentsDigest, MCPPresenter: g.Presenter, RegisteredClaims: jwt.RegisteredClaims{Subject: "User:" + g.UserID, Issuer: s.issuer, Audience: jwt.ClaimStrings{g.Audience}, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(end), ID: id}}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	token.Header["kid"] = s.kid
	return token.SignedString(s.key)
}

// Validate supplements signature/issuer/audience verification, never replaces it.
func Validate(c *internaljwt.Claims, audience string) (Grant, error) {
	if c == nil || c.AuthSource != Source || c.Actor == nil || c.PrincipalType != "user" || len(c.Audience) != 1 || c.Audience[0] != audience || c.IssuedAt == nil || c.NotBefore == nil || c.ExpiresAt == nil || c.ID == "" {
		return Grant{}, ErrInvalid
	}
	user, ok := strings.CutPrefix(c.Subject, "User:")
	if !ok {
		return Grant{}, ErrInvalid
	}
	g := Grant{Principal: Principal{c.MCPPrincipalKind, user, c.Actor.Sub, c.CredentialID, c.OrganizationID, c.WorkspaceID}, Audience: audience, CallID: c.MCPCallID, SourceID: c.MCPSourceID, Operation: c.MCPOperation, Method: c.MCPMethod, ArgumentsDigest: c.MCPArgumentsDigest, Presenter: c.MCPPresenter, ExpiresAt: c.ExpiresAt.Time}
	lifetime := c.ExpiresAt.Sub(c.IssuedAt.Time)
	if !validGrant(g) || lifetime <= 0 || lifetime > MaxLifetime || !c.NotBefore.Equal(c.IssuedAt.Time) {
		return Grant{}, ErrInvalid
	}
	return g, nil
}
func Verify(ctx context.Context, v *internaljwt.Verifier, raw, audience string) (Grant, error) {
	if v == nil {
		return Grant{}, ErrInvalid
	}
	c, err := v.Verify(ctx, raw)
	if err != nil {
		return Grant{}, ErrInvalid
	}
	return Validate(c, audience)
}
func SameInvocation(a, b Grant) bool {
	return a.Principal == b.Principal && a.CallID == b.CallID && a.SourceID == b.SourceID && a.Operation == b.Operation && !b.ExpiresAt.After(a.ExpiresAt)
}
func ServiceAuthenticated(actual, expected string) bool {
	return len(expected) >= 32 && subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

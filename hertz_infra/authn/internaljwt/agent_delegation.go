package internaljwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const AgentDelegationTTL = time.Minute
const AgentAuthSource = "agent_api_token"

var ErrAgentDelegation = errors.New("invalid Agent delegation")

type AgentPrincipal struct{ UserID, ActorID, OrganizationID, CredentialID string }
type AgentRequest struct {
	Method, URI string
	Body        []byte
}

type AgentSigner struct {
	key         *rsa.PrivateKey
	issuer, kid string
	now         func() time.Time
}

func NewAgentSigner(key *rsa.PrivateKey, issuer string, now func() time.Time) (*AgentSigner, error) {
	if key == nil || key.N == nil || key.N.BitLen() < 2048 || strings.TrimSpace(issuer) == "" {
		return nil, ErrAgentDelegation
	}
	if err := key.Validate(); err != nil {
		return nil, ErrAgentDelegation
	}
	if now == nil {
		now = time.Now
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, ErrAgentDelegation
	}
	digest := sha256.Sum256(public)
	return &AgentSigner{key: key, issuer: issuer, kid: "agent-" + hex.EncodeToString(digest[:16]), now: now}, nil
}

// ParseAgentSigningKey accepts one unencrypted private key, never a public key
// or a bundle. Errors intentionally do not include PEM bytes or file contents.
func ParseAgentSigningKey(raw []byte) (*rsa.PrivateKey, error) {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return nil, ErrAgentDelegation
	}
	block, rest := pem.Decode(raw)
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 || len(block.Headers) != 0 {
		return nil, ErrAgentDelegation
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, ErrAgentDelegation
		}
		return key, nil
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, ErrAgentDelegation
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, ErrAgentDelegation
		}
		return key, nil
	default:
		return nil, ErrAgentDelegation
	}
}

// LoadAgentSigner refuses public-readable keys and oversized/non-regular files.
// No source path or key material is exposed through errors.
func LoadAgentSigner(path, issuer string) (*AgentSigner, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrAgentDelegation
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<10 {
		return nil, ErrAgentDelegation
	}
	raw, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return nil, ErrAgentDelegation
	}
	key, err := ParseAgentSigningKey(raw)
	if err != nil {
		return nil, ErrAgentDelegation
	}
	return NewAgentSigner(key, issuer, nil)
}

type AgentPublicKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}
type AgentJWKS struct {
	Keys []AgentPublicKey `json:"keys"`
}

func (s *AgentSigner) PublicJWKS() AgentJWKS {
	if s == nil || s.key == nil {
		return AgentJWKS{Keys: []AgentPublicKey{}}
	}
	return AgentJWKS{Keys: []AgentPublicKey{{Kty: "RSA", Use: "sig", Alg: "RS256", Kid: s.kid, N: base64.RawURLEncoding.EncodeToString(s.key.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.key.E)).Bytes())}}}
}

func (s *AgentSigner) Sign(principal AgentPrincipal, audience string, request AgentRequest) (string, error) {
	if s == nil || s.key == nil || !validAgentPrincipal(principal) || !validAgentAudience(audience) || !validAgentRequest(request) {
		return "", ErrAgentDelegation
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", ErrAgentDelegation
	}
	now := s.now().UTC().Truncate(time.Second)
	claims := Claims{PrincipalType: "user", OrganizationID: principal.OrganizationID, CredentialID: principal.CredentialID, Scope: "agent.gateway", AuthSource: AgentAuthSource, Role: "user", Actor: &Actor{Sub: principal.ActorID}, RequestMethod: request.Method, RequestURI: request.URI, RequestDigest: agentRequestDigest(request), RegisteredClaims: jwt.RegisteredClaims{Subject: "User:" + principal.UserID, Issuer: s.issuer, Audience: jwt.ClaimStrings{audience}, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(AgentDelegationTTL)), ID: hex.EncodeToString(id)}}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.kid
	raw, err := token.SignedString(s.key)
	if err != nil {
		return "", ErrAgentDelegation
	}
	return raw, nil
}

// ValidateAgentRequest supplements signature/issuer/audience/time verification.
// Call only after Verifier.Verify; it is not an authentication shortcut.
func ValidateAgentRequest(claims *Claims, request AgentRequest) (AgentPrincipal, error) {
	if claims == nil || claims.AuthSource != AgentAuthSource || claims.PrincipalType != "user" || claims.Actor == nil || claims.Scope != "agent.gateway" || claims.Role != "user" || len(claims.Audience) != 1 || !validAgentAudience(claims.Audience[0]) || claims.IssuedAt == nil || claims.NotBefore == nil || claims.ExpiresAt == nil || claims.ID == "" {
		return AgentPrincipal{}, ErrAgentDelegation
	}
	user, ok := strings.CutPrefix(claims.Subject, "User:")
	principal := AgentPrincipal{UserID: user, ActorID: claims.Actor.Sub, OrganizationID: claims.OrganizationID, CredentialID: claims.CredentialID}
	lifetime := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if !ok || !validAgentPrincipal(principal) || lifetime <= 0 || lifetime > AgentDelegationTTL || !claims.NotBefore.Time.Equal(claims.IssuedAt.Time) || !validAgentRequest(request) || claims.RequestMethod != request.Method || claims.RequestURI != request.URI {
		return AgentPrincipal{}, ErrAgentDelegation
	}
	digest := agentRequestDigest(request)
	if subtle.ConstantTimeCompare([]byte(claims.RequestDigest), []byte(digest)) != 1 {
		return AgentPrincipal{}, ErrAgentDelegation
	}
	return principal, nil
}

func validAgentAudience(audience string) bool {
	return audience == "media-workflow-agent" || audience == "media-materials-agent" || audience == "media-control-agent"
}
func validAgentPrincipal(p AgentPrincipal) bool {
	actor, ok := strings.CutPrefix(p.ActorID, "Agent:")
	return ok && validIdentityPart(actor) && validIdentityPart(p.UserID) && validIdentityPart(p.OrganizationID) && validIdentityPart(p.CredentialID)
}
func validIdentityPart(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func validAgentRequest(r AgentRequest) bool {
	switch r.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return false
	}
	if len(r.URI) == 0 || len(r.URI) > 4096 || len(r.Body) > 8<<20 || !strings.HasPrefix(r.URI, "/api/v1/") || strings.ContainsAny(r.URI, "\r\n\\#") || strings.Contains(strings.SplitN(r.URI, "?", 2)[0], "%") {
		return false
	}
	u, err := url.ParseRequestURI(r.URI)
	if err != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}
func agentRequestDigest(r AgentRequest) string {
	hash := sha256.New()
	hash.Write([]byte(r.Method))
	hash.Write([]byte{0})
	hash.Write([]byte(r.URI))
	hash.Write([]byte{0})
	hash.Write(r.Body)
	return hex.EncodeToString(hash.Sum(nil))
}

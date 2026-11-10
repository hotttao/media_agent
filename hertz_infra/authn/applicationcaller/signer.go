// Package applicationcaller signs short-lived identity assertions for registered business MCPs.
// An assertion is not a platform access token or a resource authorization decision.
package applicationcaller

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"media_agent/hertz_infra/authn/internaljwt"
	"os"
	"time"
)

const Header = "X-Platform-Caller"
const Type = "platform-caller+jwt"

var ErrInvalid = errors.New("invalid application caller")

type Identity struct{ ApplicationID, UserID, ExternalID, OrganizationID, TeamID, WorkspaceID, SessionID string }
type Claims struct {
	jwt.RegisteredClaims
	ExternalID     string `json:"ext"`
	OrganizationID string `json:"organization_id"`
	TeamID         string `json:"team_id"`
	WorkspaceID    string `json:"workspace_id"`
	SessionID      string `json:"session_id"`
}
type Signer struct {
	key    *rsa.PrivateKey
	issuer string
	public internaljwt.AgentJWKS
}

func New(key *rsa.PrivateKey, issuer string) (*Signer, error) {
	s, e := internaljwt.NewAgentSigner(key, issuer, nil)
	if e != nil {
		return nil, ErrInvalid
	}
	return &Signer{key, issuer, s.PublicJWKS()}, nil
}
func Load(path, issuer string) (*Signer, error) {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return nil, ErrInvalid
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, ErrInvalid
	}
	key, e := internaljwt.ParseAgentSigningKey(raw)
	if e != nil {
		return nil, ErrInvalid
	}
	return New(key, issuer)
}
func (s *Signer) PublicJWKS() internaljwt.AgentJWKS { return s.public }
func (s *Signer) Sign(p Identity, expires time.Time) (string, error) {
	if s == nil || p.ApplicationID == "" || p.UserID == "" || p.ExternalID == "" || p.OrganizationID == "" || p.TeamID == "" || p.WorkspaceID == "" || p.SessionID == "" {
		return "", ErrInvalid
	}
	now := time.Now().UTC()
	end := now.Add(time.Minute)
	if !expires.IsZero() && expires.Before(end) {
		end = expires
	}
	if !end.After(now) {
		return "", ErrInvalid
	}
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return "", e
	}
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: s.issuer, Subject: p.UserID, Audience: jwt.ClaimStrings{p.ApplicationID}, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(end), ID: hex.EncodeToString(nonce[:])}, ExternalID: p.ExternalID, OrganizationID: p.OrganizationID, TeamID: p.TeamID, WorkspaceID: p.WorkspaceID, SessionID: p.SessionID}
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	t.Header["kid"] = s.public.Keys[0].Kid
	t.Header["typ"] = Type
	return t.SignedString(s.key)
}

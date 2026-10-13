package runtimeauth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"time"
)

var ErrSignature = errors.New("invalid runtime signing configuration")

type Scope struct {
	WorkspaceID    string `json:"workspace_id"`
	AgentConfigID  string `json:"agent_config_id"`
	ReplicaID      string `json:"replica_id"`
	IncarnationID  string `json:"incarnation_id"`
	ExecutionEpoch int64  `json:"execution_epoch"`
}
type Signer struct {
	key ed25519.PrivateKey
	kid string
}

func New(key ed25519.PrivateKey) (*Signer, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, ErrSignature
	}
	public := key.Public().(ed25519.PublicKey)
	sum := sha256.Sum256(public)
	return &Signer{append(ed25519.PrivateKey(nil), key...), "rt-" + hex.EncodeToString(sum[:8])}, nil
}
func Load(path string) (*Signer, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<10 {
		return nil, ErrSignature
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrSignature
	}
	block, rest := pem.Decode(raw)
	if block == nil || len(rest) > 0 {
		return nil, ErrSignature
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, ErrSignature
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, ErrSignature
	}
	return New(key)
}
func (s *Signer) PublicKeys() map[string]string {
	return map[string]string{s.kid: base64.RawURLEncoding.EncodeToString(s.key.Public().(ed25519.PublicKey))}
}
func (s *Signer) Sign(scope Scope, method, path string, body []byte) (string, error) {
	if scope.WorkspaceID == "" || scope.AgentConfigID == "" || scope.ReplicaID == "" || scope.IncarnationID == "" || scope.ExecutionEpoch < 0 {
		return "", ErrSignature
	}
	hash := sha256.Sum256(body)
	now := time.Now().Unix()
	value := struct {
		Scope
		Kid      string `json:"kid"`
		Audience string `json:"aud"`
		Method   string `json:"method"`
		Path     string `json:"path"`
		BodyHash string `json:"body_sha256"`
		Issued   int64  `json:"iat"`
		Expires  int64  `json:"exp"`
	}{scope, s.kid, "media-agent-runtime", method, path, hex.EncodeToString(hash[:]), now, now + 60}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", ErrSignature
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(s.key, []byte(encoded))), nil
}

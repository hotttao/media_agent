// Package session 是 SessionSigner 的 HS256 JWT 实现（共享基础设施）。
//
// 实现 biz/domain/auth.SessionSigner 接口；domain 只定义接口，本包提供具体实现
// （依赖 config.AuthConfig.JwtSecret + JWT 库，domain 不依赖这些）。
// middleware 与 service 都用本实现。
//
// 对齐 control-plane services/auth.ts 的 generateToken/verifyToken/shouldRenewToken/renewToken。
package session

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_auth/biz/domain/auth"
)

// jwtClaims 是 JWT 的 claims（对齐 control-plane JwtPayload）。
type jwtClaims struct {
	Sub         string `json:"sub"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	Email       string `json:"email,omitempty"`
	Role        string `json:"role"`
	AuthVersion int64  `json:"auth_version"`
	jwt.RegisteredClaims
}

// Signer 是 auth.SessionSigner 的 HS256 实现。
type Signer struct {
	secret       []byte
	privateKey   *rsa.PrivateKey
	publicKey    *rsa.PublicKey
	keyID        string
	acceptLegacy bool
	expiresIn    time.Duration
	accessTTL    time.Duration // OAuth2 access token TTL（默认 1h）
}

// NewSigner 按 AuthConfig 构造 Signer。jwt_secret 为空返回 error（启动期 fatal）。
func NewSigner(cfg *configpb.AuthConfig) (*Signer, error) {
	if cfg == nil {
		return nil, errors.New("session: auth config is required")
	}
	exp := time.Duration(cfg.GetJwtExpiresInSeconds()) * time.Second
	if exp <= 0 {
		exp = 24 * time.Hour // 默认 24h
	}
	accessTTL := time.Duration(cfg.GetAccessTokenExpiresInSeconds()) * time.Second
	if accessTTL <= 0 {
		accessTTL = time.Hour // 默认 1h
	}
	s := &Signer{secret: []byte(cfg.GetJwtSecret()), expiresIn: exp, accessTTL: accessTTL, keyID: cfg.GetJwtKeyId(), acceptLegacy: cfg.GetAcceptLegacyHs256()}
	if s.keyID == "" {
		s.keyID = "media-auth-1"
	}
	if cfg.GetJwtPrivateKeyFile() != "" {
		key, err := readRSAPrivateKey(cfg.GetJwtPrivateKeyFile())
		if err != nil {
			return nil, err
		}
		s.privateKey, s.publicKey = key, &key.PublicKey
		if cfg.GetJwtPublicKeyFile() != "" {
			pub, err := readRSAPublicKey(cfg.GetJwtPublicKeyFile())
			if err != nil {
				return nil, err
			}
			s.publicKey = pub
		}
	} else if len(s.secret) == 0 {
		return nil, errors.New("session: jwt_private_key_file or jwt_secret is required")
	}
	return s, nil
}

// AccessTokenTTL 返回 OAuth2 access token TTL（供 service 读取填 expires_in）。
func (s *Signer) AccessTokenTTL() time.Duration { return s.accessTTL }

// Sign 签发 session 为 HS256 JWT。
func (s *Signer) Sign(sess auth.Session) (string, error) {
	now := time.Now()
	claims := jwtClaims{
		Sub:         sess.Sub,
		Username:    sess.Username,
		Name:        sess.Name,
		Email:       sess.Email,
		Role:        sess.Role,
		AuthVersion: sess.AuthVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiresIn)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	return s.signClaims(claims)
}

// Verify 验签 token 还原 Session。
func (s *Signer) Verify(token string) (auth.Session, error) {
	claims := &jwtClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		switch t.Method.Alg() {
		case jwt.SigningMethodRS256.Alg():
			if s.publicKey == nil {
				return nil, errors.New("RS256 verification is not configured")
			}
			return s.publicKey, nil
		case jwt.SigningMethodHS256.Alg():
			if s.privateKey != nil && !s.acceptLegacy {
				return nil, errors.New("legacy HS256 verification is disabled")
			}
			if len(s.secret) == 0 {
				return nil, errors.New("legacy HS256 secret is not configured")
			}
			return s.secret, nil
		default:
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
	})
	if err != nil {
		return auth.Session{}, err
	}
	exp := time.Time{}
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Time
	}
	return auth.Session{
		Sub:         claims.Sub,
		Username:    claims.Username,
		Name:        claims.Name,
		Email:       claims.Email,
		Role:        claims.Role,
		AuthVersion: claims.AuthVersion,
		Exp:         exp,
	}, nil
}

// ShouldRenew 剩余时间 < 半周期则需续期（滑动续期，对齐 control-plane JWT_REFRESH_THRESHOLD）。
func (s *Signer) ShouldRenew(sess auth.Session) bool {
	if sess.Exp.IsZero() {
		return false
	}
	return time.Until(sess.Exp) < s.expiresIn/2
}

// Renew 用原 session 重新签发新 exp 的 token。
func (s *Signer) Renew(sess auth.Session) (string, error) {
	return s.Sign(sess)
}

// IssueAccessToken 签发 OAuth2 access token（HS256 JWT，accessTTL 有效期，与 session 同 claims 同密钥）。
//
// 供 oauth domain 的 service 调用（spec §5.6）；与 cookie session JWT 同算法同密钥、用途不同
// （access token 1h bearer，供第三方 RS 访问 /api/oauth/userinfo）。
func (s *Signer) IssueAccessToken(sess auth.Session) (string, error) {
	now := time.Now()
	claims := jwtClaims{
		Sub:         sess.Sub,
		Username:    sess.Username,
		Name:        sess.Name,
		Email:       sess.Email,
		Role:        sess.Role,
		AuthVersion: sess.AuthVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	return s.signClaims(claims)
}

type JSONWebKey struct{ Kty, Use, Alg, Kid, N, E string }

func (s *Signer) JWK() *JSONWebKey {
	if s.publicKey == nil {
		return nil
	}
	e := make([]byte, 4)
	binary.BigEndian.PutUint32(e, uint32(s.publicKey.E))
	e = bytesTrimLeftZero(e)
	return &JSONWebKey{Kty: "RSA", Use: "sig", Alg: "RS256", Kid: s.keyID,
		N: base64.RawURLEncoding.EncodeToString(s.publicKey.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(e)}
}

func (s *Signer) signClaims(claims jwt.Claims) (string, error) {
	if s.privateKey != nil {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = s.keyID
		return tok.SignedString(s.privateKey)
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func readRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session: read private key: %w", err)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("session: invalid private key PEM")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("session: parse private key: %w", err)
	}
	return key, nil
}

func readRSAPublicKey(path string) (*rsa.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session: read public key: %w", err)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("session: invalid public key PEM")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("session: parse public key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("session: public key is not RSA")
	}
	return rsaKey, nil
}

func bytesTrimLeftZero(value []byte) []byte { return new(big.Int).SetBytes(value).Bytes() }

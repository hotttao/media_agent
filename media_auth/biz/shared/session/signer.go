// Package session 是 SessionSigner 的 HS256 JWT 实现（共享基础设施）。
//
// 实现 biz/domain/auth.SessionSigner 接口；domain 只定义接口，本包提供具体实现
// （依赖 config.AuthConfig.JwtSecret + JWT 库，domain 不依赖这些）。
// middleware 与 service 都用本实现。
//
// 对齐 control-plane services/auth.ts 的 generateToken/verifyToken/shouldRenewToken/renewToken。
package session

import (
	"errors"
	"fmt"
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
	secret    []byte
	expiresIn time.Duration
	accessTTL time.Duration // OAuth2 access token TTL（默认 1h）
}

// NewSigner 按 AuthConfig 构造 Signer。jwt_secret 为空返回 error（启动期 fatal）。
func NewSigner(cfg *configpb.AuthConfig) (*Signer, error) {
	if cfg == nil || cfg.GetJwtSecret() == "" {
		return nil, errors.New("session: jwt_secret is required")
	}
	exp := time.Duration(cfg.GetJwtExpiresInSeconds()) * time.Second
	if exp <= 0 {
		exp = 24 * time.Hour // 默认 24h
	}
	accessTTL := time.Duration(cfg.GetAccessTokenExpiresInSeconds()) * time.Second
	if accessTTL <= 0 {
		accessTTL = time.Hour // 默认 1h
	}
	return &Signer{secret: []byte(cfg.GetJwtSecret()), expiresIn: exp, accessTTL: accessTTL}, nil
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
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.secret)
}

// Verify 验签 token 还原 Session。
func (s *Signer) Verify(token string) (auth.Session, error) {
	claims := &jwtClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
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
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.secret)
}

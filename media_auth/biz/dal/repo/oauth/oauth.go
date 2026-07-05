// Package oauth 是 oauth 领域仓储的 dal 实现（oauth_clients / oauth_authorization_codes /
// oauth_refresh_tokens 三表 + users 只读）。
// 实现 biz/domain/oauth.OAuthRepo；唯一可 import biz/dal/query 的位置。
//
// 事务：所有方法经 dal.FromContext(ctx, db) 取连接，自动加入上层事务（不开自身事务）。
// ConsumeAuthCode / ConsumeRefreshToken 用 DELETE ... RETURNING（GORM Raw + Scan）保证
// 「消费即删」原子性，避免 select-then-delete 的并发重放。
package oauth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	"media_agent/media_auth/biz/dal/query"
	domainoauth "media_agent/media_auth/biz/domain/oauth"
)

// NewRepo 构造 oauth 仓储。
func NewRepo(db *gorm.DB) domainoauth.OAuthRepo {
	return &repo{db: db}
}

type repo struct {
	db *gorm.DB
}

func (r *repo) q(ctx context.Context) *query.Query {
	return query.Use(dal.FromContext(ctx, r.db))
}

// GetClient 按 id 查 client。
func (r *repo) GetClient(ctx context.Context, id string) (domainoauth.Client, error) {
	q := r.q(ctx)
	row, err := q.OAuthClient.WithContext(ctx).Where(q.OAuthClient.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainoauth.Client{}, domainoauth.ErrClientNotFound
		}
		return domainoauth.Client{}, err
	}
	return rowToClient(row), nil
}

// CreateAuthCode 插入授权码（code/scope/expires_at 由 service 生成）。
func (r *repo) CreateAuthCode(ctx context.Context, ac domainoauth.AuthCode) error {
	q := r.q(ctx)
	row := model.OAuthAuthorizationCode{
		Code:          ac.Code,
		ClientID:      ac.ClientID,
		UserID:        ac.UserID,
		RedirectURI:   ac.RedirectURI,
		CodeChallenge: strPtr(ac.CodeChallenge),
		Scope:         ac.Scope,
		ExpiresAt:     ac.ExpiresAt,
	}
	return q.OAuthAuthorizationCode.WithContext(ctx).
		Omit(q.OAuthAuthorizationCode.CreatedAt).Create(&row)
}

// ConsumeAuthCode 消费授权码：DELETE WHERE code AND expires_at>now RETURNING。
func (r *repo) ConsumeAuthCode(ctx context.Context, code string) (domainoauth.AuthCode, error) {
	var row model.OAuthAuthorizationCode
	err := dal.FromContext(ctx, r.db).Raw(
		`DELETE FROM media_auth.oauth_authorization_codes
		  WHERE code = ? AND expires_at > now() RETURNING *`,
		code,
	).Scan(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || row.Code == "" {
			return domainoauth.AuthCode{}, domainoauth.ErrAuthCodeNotFound
		}
		return domainoauth.AuthCode{}, err
	}
	if row.Code == "" {
		return domainoauth.AuthCode{}, domainoauth.ErrAuthCodeNotFound
	}
	return rowToAuthCode(&row), nil
}

// CreateRefreshToken 存刷新令牌（tokenHash 由 service 算）。
func (r *repo) CreateRefreshToken(ctx context.Context, rt domainoauth.RefreshToken) error {
	q := r.q(ctx)
	row := model.OAuthRefreshToken{
		TokenHash: rt.TokenHash,
		ClientID:  rt.ClientID,
		UserID:    rt.UserID,
		ExpiresAt: rt.ExpiresAt,
	}
	return q.OAuthRefreshToken.WithContext(ctx).
		Omit(q.OAuthRefreshToken.CreatedAt).Create(&row)
}

// ConsumeRefreshToken 消费刷新令牌：DELETE WHERE token_hash AND expires_at>now RETURNING。
func (r *repo) ConsumeRefreshToken(ctx context.Context, tokenHash string) (domainoauth.RefreshToken, error) {
	var row model.OAuthRefreshToken
	err := dal.FromContext(ctx, r.db).Raw(
		`DELETE FROM media_auth.oauth_refresh_tokens
		  WHERE token_hash = ? AND expires_at > now() RETURNING *`,
		tokenHash,
	).Scan(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || row.TokenHash == "" {
			return domainoauth.RefreshToken{}, domainoauth.ErrRefreshTokenNotFound
		}
		return domainoauth.RefreshToken{}, err
	}
	if row.TokenHash == "" {
		return domainoauth.RefreshToken{}, domainoauth.ErrRefreshTokenNotFound
	}
	return rowToRefreshToken(&row), nil
}

// GetUserByID 按 id 查 user（access token claims 用）。
func (r *repo) GetUserByID(ctx context.Context, id string) (domainoauth.User, error) {
	q := r.q(ctx)
	u, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainoauth.User{}, domainoauth.ErrUserNotFound
		}
		return domainoauth.User{}, err
	}
	d := domainoauth.User{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Role:        u.Role,
	}
	if u.Email != nil {
		d.Email = *u.Email
	}
	return d, nil
}

// CleanupExpired 删过期 auth_code + refresh_token。
func (r *repo) CleanupExpired(ctx context.Context) error {
	q := r.q(ctx)
	if _, err := q.OAuthAuthorizationCode.WithContext(ctx).
		Where(q.OAuthAuthorizationCode.ExpiresAt.Lt(time.Now())).Delete(); err != nil {
		return err
	}
	_, err := q.OAuthRefreshToken.WithContext(ctx).
		Where(q.OAuthRefreshToken.ExpiresAt.Lt(time.Now())).Delete()
	return err
}

// ── row → domain 转换 ────────────────────────────────────────────────

func rowToClient(c *model.OAuthClient) domainoauth.Client {
	d := domainoauth.Client{
		ID:           c.ID,
		Name:         c.Name,
		RedirectURIs: []string(c.RedirectURIs),
		CreatedBy:    c.CreatedBy,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
	if c.SecretHash != nil {
		d.SecretHash = *c.SecretHash
	}
	return d
}

func rowToAuthCode(a *model.OAuthAuthorizationCode) domainoauth.AuthCode {
	d := domainoauth.AuthCode{
		Code:        a.Code,
		ClientID:    a.ClientID,
		UserID:      a.UserID,
		RedirectURI: a.RedirectURI,
		Scope:       a.Scope,
		ExpiresAt:   a.ExpiresAt,
		CreatedAt:   a.CreatedAt,
	}
	if a.CodeChallenge != nil {
		d.CodeChallenge = *a.CodeChallenge
	}
	return d
}

func rowToRefreshToken(rt *model.OAuthRefreshToken) domainoauth.RefreshToken {
	return domainoauth.RefreshToken{
		TokenHash: rt.TokenHash,
		ClientID:  rt.ClientID,
		UserID:    rt.UserID,
		ExpiresAt: rt.ExpiresAt,
		CreatedAt: rt.CreatedAt,
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

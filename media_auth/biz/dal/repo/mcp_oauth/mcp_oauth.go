// Package mcpoauth 是 mcp_oauth 领域仓储的 dal 实现（mcp_oauth_clients + mcp_oauth_tokens 两表）。
// 实现 biz/domain/mcp_oauth.McpRepo；唯一可 import biz/dal/query 的位置。
//
// 事务：所有方法经 dal.FromContext(ctx, db) 取连接，自动加入上层事务（不开自身事务）。
// UpsertToken 用 ON CONFLICT (user_id, server_origin) DO UPDATE，COALESCE 保留旧 refresh_token，
// 重置 refresh_fail_count=0 / refresh_fail_first_at=NULL（成功 refresh 重置计数）。
package mcpoauth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	"media_agent/media_auth/biz/dal/query"
	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
)

// NewRepo 构造 mcp_oauth 仓储。
func NewRepo(db *gorm.DB) domainmcp.McpRepo {
	return &repo{db: db}
}

type repo struct {
	db *gorm.DB
}

func (r *repo) q(ctx context.Context) *query.Query {
	return query.Use(dal.FromContext(ctx, r.db))
}

// UpsertClient upsert mcp_oauth_clients（按 server_origin）。
func (r *repo) UpsertClient(ctx context.Context, c domainmcp.McpClient) error {
	q := r.q(ctx)
	row := model.McpOAuthClient{
		ServerOrigin: c.ServerOrigin,
		Metadata:     datatypes.JSON(domainmcp.MetadataJSON(c.Metadata)),
		ClientID:     c.ClientID,
		ClientSecret: strPtr(c.ClientSecret),
	}
	return q.McpOAuthClient.WithContext(ctx).
		Omit(q.McpOAuthClient.CreatedAt, q.McpOAuthClient.UpdatedAt).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "server_origin"}},
			DoUpdates: clause.AssignmentColumns([]string{"metadata", "client_id", "client_secret", "updated_at"}),
		}).Create(&row)
}

// GetClient 按 server_origin 查 client。
func (r *repo) GetClient(ctx context.Context, serverOrigin string) (domainmcp.McpClient, error) {
	q := r.q(ctx)
	row, err := q.McpOAuthClient.WithContext(ctx).Where(q.McpOAuthClient.ServerOrigin.Eq(serverOrigin)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainmcp.McpClient{}, domainmcp.ErrClientNotFound
		}
		return domainmcp.McpClient{}, err
	}
	return rowToClient(row), nil
}

// UpsertToken upsert mcp_oauth_tokens，COALESCE 保留旧 refresh_token，重置失败计数。
func (r *repo) UpsertToken(ctx context.Context, userID, serverOrigin string, resp domainmcp.TokenResponse, ttlSeconds int) error {
	q := r.q(ctx)
	if ttlSeconds <= 0 {
		ttlSeconds = domainmcp.DefaultTokenTTLSeconds
	}
	expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	tokenType := resp.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	row := model.McpOAuthToken{
		UserID:             userID,
		ServerOrigin:       serverOrigin,
		AccessToken:        resp.AccessToken,
		RefreshToken:       strPtr(resp.RefreshToken),
		TokenType:          tokenType,
		Scope:              strPtr(resp.Scope),
		ExpiresAt:          &expiresAt,
		RefreshFailCount:   0,
		RefreshFailFirstAt: nil,
	}
	// ON CONFLICT (user_id, server_origin) DO UPDATE：access/refresh(COALESCE)/type/scope/expires/updated_at + 重置 fail 计数。
	// 用 raw SQL 表达 COALESCE + 重置（gen OnConflict 的 DoUpdates 不便写 COALESCE）。
	refreshExpr := "COALESCE(EXCLUDED.refresh_token, mcp_oauth_tokens.refresh_token)"
	res := dal.FromContext(ctx, r.db).Exec(`
		INSERT INTO media_auth.mcp_oauth_tokens
		  (user_id, server_origin, access_token, refresh_token, token_type, scope, expires_at, refresh_fail_count, refresh_fail_first_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, NULL)
		ON CONFLICT (user_id, server_origin) DO UPDATE SET
		  access_token = EXCLUDED.access_token,
		  refresh_token = `+refreshExpr+`,
		  token_type = EXCLUDED.token_type,
		  scope = EXCLUDED.scope,
		  expires_at = EXCLUDED.expires_at,
		  updated_at = now(),
		  refresh_fail_count = 0,
		  refresh_fail_first_at = NULL`,
		row.UserID, row.ServerOrigin, row.AccessToken, row.RefreshToken, row.TokenType, row.Scope, row.ExpiresAt,
	)
	_ = q // 保留 q 引用以避免未使用告警（实际走 raw SQL）
	return res.Error
}

// GetToken 取 user 对 server_origin 的 token。
func (r *repo) GetToken(ctx context.Context, userID, serverOrigin string) (domainmcp.McpToken, error) {
	q := r.q(ctx)
	row, err := q.McpOAuthToken.WithContext(ctx).
		Where(q.McpOAuthToken.UserID.Eq(userID), q.McpOAuthToken.ServerOrigin.Eq(serverOrigin)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainmcp.McpToken{}, domainmcp.ErrTokenNotFound
		}
		return domainmcp.McpToken{}, err
	}
	return rowToToken(row), nil
}

// ListTokensForUser 列 user 的全部 token。
func (r *repo) ListTokensForUser(ctx context.Context, userID string) ([]domainmcp.McpToken, error) {
	q := r.q(ctx)
	rows, err := q.McpOAuthToken.WithContext(ctx).Where(q.McpOAuthToken.UserID.Eq(userID)).Find()
	if err != nil {
		return nil, err
	}
	out := make([]domainmcp.McpToken, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToToken(row))
	}
	return out, nil
}

// RecordRefreshFailure 失败计数 +1，首次失败盖章。返回当前计数 + 首次时间。
//
// 用 UpdateColumns（gorm.Expr 表达 COALESCE）+ 回读，避免 GORM Raw 对 UPDATE...RETURNING 的解析问题。
func (r *repo) RecordRefreshFailure(ctx context.Context, userID, serverOrigin string) (int, *time.Time, error) {
	q := r.q(ctx)
	if _, err := q.McpOAuthToken.WithContext(ctx).
		Where(q.McpOAuthToken.UserID.Eq(userID), q.McpOAuthToken.ServerOrigin.Eq(serverOrigin)).
		UpdateColumns(map[string]any{
			"refresh_fail_count":    gorm.Expr("refresh_fail_count + 1"),
			"refresh_fail_first_at": gorm.Expr("COALESCE(refresh_fail_first_at, now())"),
		}); err != nil {
		return 0, nil, err
	}
	tok, err := r.GetToken(ctx, userID, serverOrigin)
	if err != nil {
		// token 行不存在（已删）→ 返回 0/nil。
		return 0, nil, nil
	}
	return tok.RefreshFailCount, tok.RefreshFailFirstAt, nil
}

// DeleteToken 删 token。
func (r *repo) DeleteToken(ctx context.Context, userID, serverOrigin string) error {
	q := r.q(ctx)
	_, err := q.McpOAuthToken.WithContext(ctx).
		Where(q.McpOAuthToken.UserID.Eq(userID), q.McpOAuthToken.ServerOrigin.Eq(serverOrigin)).Delete()
	return err
}

// ── row → domain 转换 ────────────────────────────────────────────────

func rowToClient(c *model.McpOAuthClient) domainmcp.McpClient {
	d := domainmcp.McpClient{
		ServerOrigin: c.ServerOrigin,
		Metadata:     domainmcp.ParseMetadata(json.RawMessage(c.Metadata)),
		ClientID:     c.ClientID,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
	if c.ClientSecret != nil {
		d.ClientSecret = *c.ClientSecret
	}
	return d
}

func rowToToken(t *model.McpOAuthToken) domainmcp.McpToken {
	d := domainmcp.McpToken{
		UserID:             t.UserID,
		ServerOrigin:       t.ServerOrigin,
		AccessToken:        t.AccessToken,
		TokenType:          t.TokenType,
		RefreshFailCount:   t.RefreshFailCount,
		ExpiresAt:          t.ExpiresAt,
		RefreshFailFirstAt: t.RefreshFailFirstAt,
		CreatedAt:          t.CreatedAt,
		UpdatedAt:          t.UpdatedAt,
	}
	if t.RefreshToken != nil {
		d.RefreshToken = *t.RefreshToken
	}
	if t.Scope != nil {
		d.Scope = *t.Scope
	}
	return d
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

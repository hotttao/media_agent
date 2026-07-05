// Package servicetoken 是 service_token 领域仓储的 dal 实现（service_tokens 表）。
// 实现 biz/domain/service_token.TokenRepo；唯一可 import biz/dal/query 的位置。
// 把 DB row model 转为 domain 对象，禁止把 GORM 行模型返回给上层。
//
// 事务：所有方法经 dal.FromContext(ctx, db) 取连接，自动加入上层事务（不开自身事务）。
package servicetoken

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	"media_agent/media_auth/biz/dal/query"
	domainst "media_agent/media_auth/biz/domain/service_token"
	"media_agent/media_auth/biz/shared/ids"
	"media_agent/media_auth/biz/shared/token"
)

// NewRepo 构造 service_token 仓储。
func NewRepo(db *gorm.DB) domainst.TokenRepo {
	return &repo{db: db}
}

type repo struct {
	db *gorm.DB
}

func (r *repo) q(ctx context.Context) *query.Query {
	return query.Use(dal.FromContext(ctx, r.db))
}

// Create 新建非 platform token 行。
func (r *repo) Create(ctx context.Context, name, tokenHash, createdBy string) (domainst.Token, error) {
	q := r.q(ctx)
	row := model.ServiceToken{
		ID:         ids.New(),
		Name:       name,
		TokenHash:  tokenHash,
		CreatedBy:  &createdBy,
		IsPlatform: false,
	}
	if err := q.ServiceToken.WithContext(ctx).Omit(q.ServiceToken.RevokedAt).Create(&row); err != nil {
		return domainst.Token{}, err
	}
	return rowToDomain(&row), nil
}

// GetByHash 按 token_hash 查未撤销 token（VerifyToken 用）。
func (r *repo) GetByHash(ctx context.Context, tokenHash string) (domainst.Token, error) {
	q := r.q(ctx)
	row, err := q.ServiceToken.WithContext(ctx).
		Where(q.ServiceToken.TokenHash.Eq(tokenHash), q.ServiceToken.RevokedAt.IsNull()).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainst.Token{}, domainst.ErrNotFound
		}
		return domainst.Token{}, err
	}
	return rowToDomain(row), nil
}

// ListByUser 列 user 的全部未撤销 token（不含 hash/明文），按 is_platform ASC, created_at DESC。
func (r *repo) ListByUser(ctx context.Context, userID string) ([]domainst.Token, error) {
	q := r.q(ctx)
	rows, err := q.ServiceToken.WithContext(ctx).
		Where(q.ServiceToken.CreatedBy.Eq(userID), q.ServiceToken.RevokedAt.IsNull()).
		Order(q.ServiceToken.IsPlatform, q.ServiceToken.CreatedAt.Desc()).Find()
	if err != nil {
		return nil, err
	}
	out := make([]domainst.Token, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToDomain(row))
	}
	return out, nil
}

// Revoke 撤销：UPDATE revoked_at=now WHERE id AND revoked_at IS NULL AND is_platform=false。
func (r *repo) Revoke(ctx context.Context, id string) error {
	q := r.q(ctx)
	res, err := q.ServiceToken.WithContext(ctx).
		Where(q.ServiceToken.ID.Eq(id), q.ServiceToken.RevokedAt.IsNull(), q.ServiceToken.IsPlatform.Is(false)).
		UpdateColumn(q.ServiceToken.RevokedAt, time.Now())
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return domainst.ErrNotFound
	}
	return nil
}

// GetPlatformToken 取 user 的 active platform token 明文。不存在返回 ""。
func (r *repo) GetPlatformToken(ctx context.Context, userID string) (string, error) {
	q := r.q(ctx)
	row, err := q.ServiceToken.WithContext(ctx).
		Where(q.ServiceToken.CreatedBy.Eq(userID), q.ServiceToken.IsPlatform.Is(true), q.ServiceToken.RevokedAt.IsNull()).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	if row.Token == nil {
		return "", nil
	}
	return *row.Token, nil
}

// EnsurePlatformToken 确保 user 拥有 active platform token：存在则返回明文；不存在则建。
//
// 并发安全：DB 部分唯一索引 idx_service_tokens_platform_user 保证每用户至多一个 active platform token；
// INSERT 用 ON CONFLICT DO NOTHING，冲突后回读现有 token 明文。
func (r *repo) EnsurePlatformToken(ctx context.Context, userID string) (string, error) {
	q := r.q(ctx)
	// 1. 先查现有 active platform token。
	if existing, err := r.GetPlatformToken(ctx, userID); err != nil {
		return "", err
	} else if existing != "" {
		return existing, nil
	}
	// 2. 不存在 → 生成新 token 并插入（ON CONFLICT DO NOTHING 兜底并发）。
	plaintext, hash, err := token.GenerateTokenAndHash()
	if err != nil {
		return "", err
	}
	name := "__platform__"
	row := model.ServiceToken{
		ID:         ids.New(),
		Name:       name,
		TokenHash:  hash,
		Token:      &plaintext,
		CreatedBy:  &userID,
		IsPlatform: true,
	}
	if err := q.ServiceToken.WithContext(ctx).
		Omit(q.ServiceToken.RevokedAt).
		Clauses(clause.OnConflict{DoNothing: true}).Create(&row); err != nil {
		return "", err
	}
	// 3. 回读（并发时另一请求可能已插入，回读保证拿到唯一 active 的明文）。
	return r.GetPlatformToken(ctx, userID)
}

// ── row → domain 转换 ────────────────────────────────────────────────

func rowToDomain(row *model.ServiceToken) domainst.Token {
	t := domainst.Token{
		ID:         row.ID,
		Name:       row.Name,
		TokenHash:  row.TokenHash,
		CreatedAt:  row.CreatedAt,
		IsPlatform: row.IsPlatform,
		RevokedAt:  row.RevokedAt,
	}
	if row.Token != nil {
		t.Token = *row.Token
	}
	if row.CreatedBy != nil {
		t.CreatedBy = *row.CreatedBy
	}
	return t
}

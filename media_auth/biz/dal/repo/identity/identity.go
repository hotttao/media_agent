// Package identity 是 identity 领域仓储的 dal 实现（user_identities 表）。
// 实现 biz/domain/identity.IdentityRepo；唯一可 import biz/dal/query 的位置。
// 把 DB row model 转为 domain 对象，禁止把 GORM row model 返回给上层。
//
// 事务：所有方法经 dal.FromContext(ctx, db) 取连接，自动加入上层事务（不开自身事务）。
package identity

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	"media_agent/media_auth/biz/dal/query"
	domainidentity "media_agent/media_auth/biz/domain/identity"
)

// NewRepo 构造 identity 仓储。
func NewRepo(db *gorm.DB) domainidentity.IdentityRepo {
	return &repo{db: db}
}

type repo struct {
	db *gorm.DB
}

func (r *repo) q(ctx context.Context) *query.Query {
	return query.Use(dal.FromContext(ctx, r.db))
}

// ListByUser 列 user 的全部绑定，按 created_at ASC。
func (r *repo) ListByUser(ctx context.Context, userID string) ([]domainidentity.Identity, error) {
	q := r.q(ctx)
	rows, err := q.UserIdentity.WithContext(ctx).
		Where(q.UserIdentity.UserID.Eq(userID)).Order(q.UserIdentity.CreatedAt).Find()
	if err != nil {
		return nil, err
	}
	out := make([]domainidentity.Identity, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToDomain(row))
	}
	return out, nil
}

// FindByExternal 按 (provider, external_id) 查（PK）。不存在返回 ErrNotFound。
func (r *repo) FindByExternal(ctx context.Context, provider domainidentity.Provider, externalID string) (domainidentity.Identity, error) {
	q := r.q(ctx)
	row, err := q.UserIdentity.WithContext(ctx).
		Where(q.UserIdentity.Provider.Eq(string(provider)), q.UserIdentity.ExternalID.Eq(externalID)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainidentity.Identity{}, domainidentity.ErrNotFound
		}
		return domainidentity.Identity{}, err
	}
	return rowToDomain(row), nil
}

// FindByUserProvider 按 (user_id, provider) 查（唯一索引）。不存在返回 ErrNotFound。
func (r *repo) FindByUserProvider(ctx context.Context, userID string, provider domainidentity.Provider) (domainidentity.Identity, error) {
	q := r.q(ctx)
	row, err := q.UserIdentity.WithContext(ctx).
		Where(q.UserIdentity.UserID.Eq(userID), q.UserIdentity.Provider.Eq(string(provider))).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainidentity.Identity{}, domainidentity.ErrNotFound
		}
		return domainidentity.Identity{}, err
	}
	return rowToDomain(row), nil
}

// Create 新建绑定。违反 (provider,external_id) 唯一 → ErrAlreadyBound。
func (r *repo) Create(ctx context.Context, id domainidentity.Identity) (domainidentity.Identity, error) {
	q := r.q(ctx)
	row := model.UserIdentity{
		UserID:      id.UserID,
		Provider:    string(id.Provider),
		ExternalID:  id.ExternalID,
		DisplayName: strPtr(id.DisplayName),
		Metadata:    toJSON(id.Metadata),
	}
	if err := q.UserIdentity.WithContext(ctx).Create(&row); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domainidentity.Identity{}, domainidentity.ErrAlreadyBound
		}
		return domainidentity.Identity{}, err
	}
	return rowToDomain(&row), nil
}

// Upsert 按 (user_id, provider) upsert（ON CONFLICT (user_id, provider) DO UPDATE）。
//
// PK 是 (provider, external_id)。service 层在调用前用 FindByExternal 预检：若 (provider,
// external_id) 已属别人则 ErrAlreadyBound。此处仍可能因并发遇到 PK 冲突 → ErrAlreadyBound。
func (r *repo) Upsert(ctx context.Context, id domainidentity.Identity) (domainidentity.Identity, error) {
	q := r.q(ctx)
	row := model.UserIdentity{
		UserID:      id.UserID,
		Provider:    string(id.Provider),
		ExternalID:  id.ExternalID,
		DisplayName: strPtr(id.DisplayName),
		Metadata:    toJSON(id.Metadata),
	}
	err := q.UserIdentity.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "provider"}},
		DoUpdates: clause.AssignmentColumns([]string{"external_id", "display_name", "metadata"}),
	}).Create(&row)
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domainidentity.Identity{}, domainidentity.ErrAlreadyBound
		}
		return domainidentity.Identity{}, err
	}
	// 回读保证 created_at 等字段完整（ON CONFLICT DO UPDATE 不返回完整行）。
	return r.FindByUserProvider(ctx, id.UserID, id.Provider)
}

// Delete 按 (user_id, provider) 删。0 行返回 ErrNotFound。
func (r *repo) Delete(ctx context.Context, userID string, provider domainidentity.Provider) error {
	q := r.q(ctx)
	res, err := q.UserIdentity.WithContext(ctx).
		Where(q.UserIdentity.UserID.Eq(userID), q.UserIdentity.Provider.Eq(string(provider))).Delete()
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return domainidentity.ErrNotFound
	}
	return nil
}

// ── row → domain 转换 ────────────────────────────────────────────────

func rowToDomain(row *model.UserIdentity) domainidentity.Identity {
	d := domainidentity.Identity{
		UserID:     row.UserID,
		Provider:   domainidentity.Provider(row.Provider),
		ExternalID: row.ExternalID,
		Metadata:   fromJSON(row.Metadata),
		CreatedAt:  row.CreatedAt,
	}
	if row.DisplayName != nil {
		d.DisplayName = *row.DisplayName
	}
	return d
}

// ── helpers ──────────────────────────────────────────────────────────

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toJSON(m json.RawMessage) datatypes.JSON {
	if len(m) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(m)
}

func fromJSON(j datatypes.JSON) json.RawMessage {
	if len(j) == 0 {
		return nil
	}
	return json.RawMessage(j)
}

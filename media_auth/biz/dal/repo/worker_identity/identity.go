package workeridentity

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/worker_identity"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(ctx context.Context, identity domain.Identity) (domain.Identity, error) {
	row := model.WorkerIdentity{ID: identity.ID, OwnerUserID: identity.OwnerUserID,
		OrganizationID: identity.OrganizationID, Name: identity.Name, Status: string(identity.Status),
		CreatedAt: identity.CreatedAt, DisabledAt: identity.DisabledAt, DeletedAt: identity.DeletedAt}
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Create(&row).Error; err != nil {
		var sqlErr interface{ SQLState() string }
		if errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &sqlErr) && sqlErr.SQLState() == "23505") {
			return domain.Identity{}, domain.ErrConflict
		}
		return domain.Identity{}, err
	}
	return toDomain(row), nil
}

func (r *Repo) Get(ctx context.Context, id string) (domain.Identity, error) {
	var row model.WorkerIdentity
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Identity{}, domain.ErrNotFound
		}
		return domain.Identity{}, err
	}
	return toDomain(row), nil
}

func (r *Repo) ListByOwner(ctx context.Context, owner string) ([]domain.Identity, error) {
	var rows []model.WorkerIdentity
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("owner_user_id = ?", owner).
		Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Identity, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomain(row))
	}
	return out, nil
}

func (r *Repo) UpdateStatus(ctx context.Context, id, owner string, old, next domain.Status, at time.Time) error {
	updates := map[string]any{"status": string(next)}
	switch next {
	case domain.StatusDisabled:
		updates["disabled_at"] = at
	case domain.StatusDeleted:
		updates["deleted_at"] = at
	}
	result := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.WorkerIdentity{}).
		Where("id = ? AND owner_user_id = ? AND status = ?", id, owner, string(old)).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repo) ListInactive(ctx context.Context, afterID string, limit int) ([]domain.Identity, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var rows []model.WorkerIdentity
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("id > ? AND status <> ?", afterID, domain.StatusActive).
		Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Identity, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomain(row))
	}
	return out, nil
}

func toDomain(row model.WorkerIdentity) domain.Identity {
	return domain.Identity{ID: row.ID, OwnerUserID: row.OwnerUserID, OrganizationID: row.OrganizationID,
		Name: row.Name, Status: domain.Status(row.Status), CreatedAt: row.CreatedAt,
		DisabledAt: row.DisabledAt, DeletedAt: row.DeletedAt}
}

var _ domain.Repository = (*Repo)(nil)

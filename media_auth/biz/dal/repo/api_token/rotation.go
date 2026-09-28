package apitoken

import (
	"context"
	"time"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/api_token"
)

func (r *Repo) BeginRotation(ctx context.Context, token domain.Token, operation domain.Rotation) error {
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	var subject any = token.SubjectUserID
	if token.SubjectUserID == "" {
		subject = nil
	}
	result := db.Model(&model.APIToken{}).
		Where("id = ? AND subject_user_id IS NOT DISTINCT FROM ? AND talos_key_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)",
			token.ID, subject, token.TalosKeyID, domain.StatusActive, operation.CreatedAt).
		Update("status", domain.StatusRotating)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrConflict
	}
	return db.Create(&model.APITokenRotation{ID: operation.ID, TokenID: token.ID, OldKeyID: token.TalosKeyID,
		ActorID: token.ActorID, Status: string(domain.RotationPending), Deadline: operation.Deadline, CreatedAt: operation.CreatedAt}).Error
}

func (r *Repo) CommitRotation(ctx context.Context, operation domain.Rotation, newKeyID string, expiry *time.Time, at time.Time) (domain.Token, error) {
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	result := db.Model(&model.APITokenRotation{}).
		Where("id = ? AND token_id = ? AND old_key_id = ? AND status = ? AND deadline > ?", operation.ID, operation.TokenID, operation.OldKeyID, domain.RotationPending, at).
		Update("status", domain.RotationCompleted)
	if result.Error != nil {
		return domain.Token{}, result.Error
	}
	if result.RowsAffected != 1 {
		return domain.Token{}, domain.ErrConflict
	}
	result = db.Model(&model.APIToken{}).
		Where("id = ? AND talos_key_id = ? AND status = ?", operation.TokenID, operation.OldKeyID, domain.StatusRotating).
		Updates(map[string]any{"talos_key_id": newKeyID, "expires_at": expiry, "rotated_at": at, "status": domain.StatusActive})
	if result.Error != nil {
		return domain.Token{}, result.Error
	}
	if result.RowsAffected != 1 {
		return domain.Token{}, domain.ErrConflict
	}
	return r.Get(ctx, operation.TokenID)
}

func (r *Repo) ClaimRotationRecovery(ctx context.Context, id string, at time.Time, force bool) (bool, error) {
	db := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.APITokenRotation{}).Where("id = ?", id)
	if force {
		db = db.Where("status IN ?", []domain.RotationStatus{domain.RotationPending, domain.RotationRecovering})
	} else {
		db = db.Where("(status = ? OR (status = ? AND deadline <= ?))", domain.RotationRecovering, domain.RotationPending, at)
	}
	result := db.Update("status", domain.RotationRecovering)
	return result.RowsAffected == 1, result.Error
}

func (r *Repo) AbortRotation(ctx context.Context, operation domain.Rotation, at time.Time) error {
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	result := db.Model(&model.APITokenRotation{}).Where("id = ? AND status = ?", operation.ID, domain.RotationRecovering).
		Update("status", domain.RotationAborted)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrConflict
	}
	result = db.Model(&model.APIToken{}).
		Where("id = ? AND talos_key_id = ? AND status IN ?", operation.TokenID, operation.OldKeyID, []domain.Status{domain.StatusRotating, domain.StatusRevoking, domain.StatusRevoked}).
		Updates(map[string]any{"status": domain.StatusRevoked, "revoked_at": at})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repo) ListPendingRotations(ctx context.Context, afterID string, at time.Time, limit int) ([]domain.Rotation, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var rows []model.APITokenRotation
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).
		Where("id > ? AND (status = ? OR (status = ? AND deadline <= ?))", afterID, domain.RotationRecovering, domain.RotationPending, at).
		Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	operations := make([]domain.Rotation, 0, len(rows))
	for _, row := range rows {
		operations = append(operations, domain.Rotation{ID: row.ID, TokenID: row.TokenID, OldKeyID: row.OldKeyID, ActorID: row.ActorID,
			Status: domain.RotationStatus(row.Status), Deadline: row.Deadline, CreatedAt: row.CreatedAt})
	}
	return operations, nil
}

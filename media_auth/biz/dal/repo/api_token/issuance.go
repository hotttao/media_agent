package apitoken

import (
	"context"
	"time"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/api_token"
)

func (r *Repo) CommitIssuance(ctx context.Context, id, keyID string, expiry *time.Time, at time.Time) (domain.Token, error) {
	if keyID == "" || expiry == nil || !expiry.After(at) {
		return domain.Token{}, domain.ErrConflict
	}
	result := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.APIToken{}).
		Where("id = ? AND status = ? AND talos_key_id IS NULL AND issuance_deadline > ?", id, domain.StatusIssuing, at).
		Updates(map[string]any{"status": domain.StatusActive, "talos_key_id": keyID, "expires_at": expiry})
	if result.Error != nil {
		return domain.Token{}, result.Error
	}
	if result.RowsAffected != 1 {
		return domain.Token{}, domain.ErrConflict
	}
	return r.Get(ctx, id)
}

func (r *Repo) ClaimIssuanceCleanup(ctx context.Context, id string, at time.Time, force bool) (domain.Token, bool, error) {
	db := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.APIToken{}).Where("id = ?", id)
	if force {
		db = db.Where("status IN ?", []domain.Status{domain.StatusIssuing, domain.StatusIssuanceCleanup})
	} else {
		db = db.Where("(status = ? OR (status = ? AND issuance_deadline <= ?))", domain.StatusIssuanceCleanup, domain.StatusIssuing, at)
	}
	result := db.Update("status", domain.StatusIssuanceCleanup)
	if result.Error != nil || result.RowsAffected != 1 {
		return domain.Token{}, false, result.Error
	}
	token, err := r.Get(ctx, id)
	return token, err == nil, err
}

func (r *Repo) FinishIssuanceCleanup(ctx context.Context, id, keyID string, at time.Time) error {
	if keyID == "" {
		return domain.ErrConflict
	}
	result := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.APIToken{}).
		Where("id = ? AND status = ? AND (talos_key_id IS NULL OR talos_key_id = ?)", id, domain.StatusIssuanceCleanup, keyID).
		Updates(map[string]any{"status": domain.StatusRevoked, "talos_key_id": keyID, "revoked_at": at})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	// Two recovery instances may finish the same confirmed revocation.
	token, err := r.Get(ctx, id)
	if err == nil && token.Status == domain.StatusRevoked && token.TalosKeyID == keyID {
		return nil
	}
	if err != nil {
		return err
	}
	return domain.ErrConflict
}

func (r *Repo) ListPendingIssuances(ctx context.Context, afterID string, at time.Time, limit int) ([]domain.Token, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var rows []model.APIToken
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).
		Where("id > ? AND (status = ? OR (status = ? AND issuance_deadline <= ?))", afterID, domain.StatusIssuanceCleanup, domain.StatusIssuing, at).
		Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	tokens := make([]domain.Token, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, toDomain(row))
	}
	return tokens, nil
}

package apitoken

import (
	"context"
	"gorm.io/gorm"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/api_token"
)

func (r *Repo) PrepareActorRevocation(ctx context.Context, kind domain.ActorType, actor, owner, organization string) error {
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	// Keep unsigned issuance in its own recovery state. Active/rotating keys
	// retain the mapping required by normal revocation and rotation recovery.
	return db.Model(&model.APIToken{}).
		Where("actor_type = ? AND actor_id = ? AND subject_user_id = ? AND organization_id = ? AND status IN ?",
			kind, actor, owner, organization, []domain.Status{domain.StatusIssuing, domain.StatusActive, domain.StatusRotating, domain.StatusExpired}).
		Update("status", gorm.Expr("CASE WHEN status = 'issuing' THEN 'issuance_cleanup' ELSE 'revoking' END")).Error
}

func (r *Repo) ListAgentRevocationCandidates(ctx context.Context, afterID string, limit int) ([]domain.Token, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var rows []model.APIToken
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).
		Where("actor_type = ? AND status IN ? AND id > ?", domain.ActorAgent,
			[]domain.Status{domain.StatusIssuing, domain.StatusActive, domain.StatusRotating, domain.StatusExpired}, afterID).
		Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	tokens := make([]domain.Token, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, toDomain(row))
	}
	return tokens, nil
}

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

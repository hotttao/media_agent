package platformtoken

import (
	"context"

	"media_agent/media_auth/biz/dal"
	domain "media_agent/media_auth/biz/domain/platform_token"
)

func (r *Repo) AppendAudit(ctx context.Context, event domain.AuditEvent) error {
	if !dal.InTransaction(ctx) || !event.Valid() {
		return domain.ErrInvalid
	}
	return dal.FromContext(ctx, r.db).WithContext(ctx).Exec(`INSERT INTO media_auth.platform_token_audit_outbox (id,user_id,credential_id,operation,caller_service,purpose,occurred_at) VALUES (?,?,?,?,?,?,?)`, event.ID, event.UserID, event.CredentialID, event.Operation, event.CallerService, event.Purpose, event.OccurredAt).Error
}

var _ domain.AuditWriter = (*Repo)(nil)

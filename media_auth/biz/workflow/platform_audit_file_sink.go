package workflow

import (
	"context"

	domain "media_agent/media_auth/biz/domain/platform_token"
	"media_agent/media_auth/biz/shared/platformaudit"
)

// PlatformAuditFileSink maps a Domain fact to the fixed infrastructure record.
// It cannot access Token ciphertext or any credential repository.
type PlatformAuditFileSink struct{ file *platformaudit.FileSink }

func OpenPlatformAuditFileSink(path string) (*PlatformAuditFileSink, error) {
	f, err := platformaudit.Open(path)
	if err != nil {
		return nil, ErrPlatformAuditDelivery
	}
	return &PlatformAuditFileSink{file: f}, nil
}
func (s *PlatformAuditFileSink) Publish(ctx context.Context, e domain.AuditEvent) error {
	if s == nil || s.file == nil || !e.Valid() {
		return ErrPlatformAuditDelivery
	}
	return s.file.Publish(ctx, platformaudit.Record{ID: e.ID, UserID: e.UserID, CredentialID: e.CredentialID, Operation: e.Operation, CallerService: e.CallerService, Purpose: e.Purpose, OccurredAt: e.OccurredAt})
}
func (s *PlatformAuditFileSink) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	return s.file.Close()
}

var _ domain.AuditSink = (*PlatformAuditFileSink)(nil)

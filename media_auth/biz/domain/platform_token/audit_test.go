package platformtoken

import (
	"testing"
	"time"
)

func TestAuditRejectsFreeFormActorPurposeAndOperations(t *testing.T) {
	base := AuditEvent{ID: "audit", UserID: "owner", CredentialID: "credential", Operation: "acquired", CallerService: CallerGateway, Purpose: PurposeOffline, OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	if !base.Valid() {
		t.Fatal("valid metadata audit rejected")
	}
	for _, mutate := range []func(*AuditEvent){func(e *AuditEvent) { e.Operation = "secret-data" }, func(e *AuditEvent) { e.CallerService = "user-header" }, func(e *AuditEvent) { e.Purpose = "arbitrary" }, func(e *AuditEvent) { e.UserID = "" }, func(e *AuditEvent) { e.ID = "id\n" }, func(e *AuditEvent) { e.OccurredAt = time.Time{} }} {
		bad := base
		mutate(&bad)
		if bad.Valid() {
			t.Fatal("invalid audit metadata accepted")
		}
	}
}

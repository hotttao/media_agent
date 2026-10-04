package platformtoken

import (
	"context"
	"time"
)

const (
	CallerGateway     = "media_agent_gateway"
	CallerSchedule    = "media_control_schedule_worker"
	CallerAuth        = "media_auth"
	PurposeOffline    = "offline-task"
	PurposeLogin      = "login-bootstrap"
	PurposeRevocation = "user-revocation"
)

// AuditEvent has no free-form payload or credential material.
type AuditEvent struct {
	ID, UserID, CredentialID, Operation, CallerService, Purpose string
	OccurredAt                                                  time.Time
}

func AllowedCaller(caller, purpose string) bool {
	return purpose == PurposeOffline && (caller == CallerGateway || caller == CallerSchedule)
}

func (e AuditEvent) Valid() bool {
	if !ValidIdentity(e.ID) || !ValidIdentity(e.UserID) || !ValidIdentity(e.CredentialID) || e.OccurredAt.IsZero() {
		return false
	}
	if AllowedCaller(e.CallerService, e.Purpose) {
		return e.Operation == "issued" || e.Operation == "acquired" || e.Operation == "introspected" || e.Operation == "rotated"
	}
	return e.CallerService == CallerAuth && (e.Purpose == PurposeLogin && e.Operation == "issued" || e.Purpose == PurposeRevocation && e.Operation == "revoked")
}

type AuditWriter interface {
	AppendAudit(context.Context, AuditEvent) error
}

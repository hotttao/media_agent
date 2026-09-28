package apitoken

import (
	"context"
	"time"
)

type RotationStatus string

const (
	RotationPending    RotationStatus = "pending"
	RotationRecovering RotationStatus = "recovering"
	RotationCompleted  RotationStatus = "completed"
	RotationAborted    RotationStatus = "aborted"
)

type Rotation struct {
	ID, TokenID, OldKeyID, ActorID string
	Status                         RotationStatus
	Deadline, CreatedAt            time.Time
}

// Multi-write methods require the caller's transaction. No external calls may
// occur inside that transaction.
type RotationRepository interface {
	BeginRotation(context.Context, Token, Rotation) error
	CommitRotation(context.Context, Rotation, string, *time.Time, time.Time) (Token, error)
	ClaimRotationRecovery(context.Context, string, time.Time, bool) (bool, error)
	AbortRotation(context.Context, Rotation, time.Time) error
	ListPendingRotations(context.Context, string, time.Time, int) ([]Rotation, error)
}

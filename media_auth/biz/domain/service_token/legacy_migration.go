package servicetoken

import (
	"context"
	"time"
)

// LegacyPlatformRecord is a transient maintenance projection, not a new
// plaintext aggregate. It must never be logged, serialized or persisted anew.
type LegacyPlatformRecord struct {
	ID, UserID, Secret, Hash string
	CreatedAt                time.Time
	ExpiresAt, RevokedAt     *time.Time
}

type PlatformMigrationSource interface {
	LockPlatformMigration(context.Context) error
	ReadPlatformMigrationPage(context.Context, string, int) ([]LegacyPlatformRecord, error)
	DeleteMigratedPlatform(context.Context, string) error
}

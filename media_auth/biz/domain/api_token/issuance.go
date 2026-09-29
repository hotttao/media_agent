package apitoken

import (
	"context"
	"time"
)

// IssuanceRepository operates on the API Token aggregate's durable creation
// intent. Multi-statement operations join the caller's local transaction.
type IssuanceRepository interface {
	Create(context.Context, Token) (Token, error)
	CommitIssuance(context.Context, string, string, *time.Time, time.Time) (Token, error)
	ClaimIssuanceCleanup(context.Context, string, time.Time, bool) (Token, bool, error)
	FinishIssuanceCleanup(context.Context, string, string, time.Time) error
	ListPendingIssuances(context.Context, string, time.Time, int) ([]Token, error)
}

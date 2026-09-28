package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"

	domain "media_agent/media_auth/biz/domain/api_token"
)

type pendingRevocations struct {
	rows []domain.Token
	err  error
}

func (p *pendingRevocations) ListRevoking(_ context.Context, afterID string, limit int) ([]domain.Token, error) {
	if p.err != nil {
		return nil, p.err
	}
	var rows []domain.Token
	for _, token := range p.rows {
		if token.ID > afterID {
			rows = append(rows, token)
			if len(rows) == limit {
				break
			}
		}
	}
	return rows, nil
}

type recoveringRevoker struct {
	owners    map[string]string
	failedKey string
}

func (r *recoveringRevoker) Revoke(ctx context.Context, owner, id string) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("attempt has no deadline")
	}
	if r.owners == nil {
		r.owners = make(map[string]string)
	}
	r.owners[id] = owner
	if id == r.failedKey {
		return errors.New("upstream unavailable")
	}
	return nil
}

func TestAPITokenRevocationPagesPastFailedKeysAndRetriesNextSweep(t *testing.T) {
	source := &pendingRevocations{}
	for i := 0; i < 101; i++ {
		source.rows = append(source.rows, domain.Token{ID: fmt.Sprintf("token-%03d", i), SubjectUserID: fmt.Sprintf("owner-%03d", i)})
	}
	revoker := &recoveringRevoker{failedKey: "token-000"}
	flow := NewAPITokenRevocation(source, revoker)
	cursor, err := flow.ProcessBatch(t.Context(), "")
	if err != nil || cursor != "token-099" || len(revoker.owners) != 100 {
		t.Fatalf("first page: %q %v", cursor, err)
	}
	cursor, err = flow.ProcessBatch(t.Context(), cursor)
	if err != nil || cursor != "" || revoker.owners["token-100"] != "owner-100" {
		t.Fatalf("failed key starved later page: %q %v", cursor, err)
	}
	revoker.owners = nil
	if _, err := flow.ProcessBatch(t.Context(), cursor); err != nil || revoker.owners["token-000"] != "owner-000" {
		t.Fatal("failed keys must be retried next sweep")
	}
}

func TestAPITokenRevocationScanFailurePreservesCursor(t *testing.T) {
	scanErr := errors.New("database unavailable")
	flow := NewAPITokenRevocation(&pendingRevocations{err: scanErr}, &recoveringRevoker{})
	cursor, err := flow.ProcessBatch(t.Context(), "last-scanned")
	if !errors.Is(err, scanErr) || cursor != "last-scanned" {
		t.Fatal("failed scan lost progress")
	}
}

func TestAPITokenRevocationCancellationDoesNotStartExternalWork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	revoker := &recoveringRevoker{}
	flow := NewAPITokenRevocation(&pendingRevocations{rows: []domain.Token{{ID: "token"}}}, revoker)
	if _, err := flow.ProcessBatch(ctx, ""); !errors.Is(err, context.Canceled) || len(revoker.owners) != 0 {
		t.Fatal("cancelled recovery started an external call")
	}
	flow.Run(ctx)
}

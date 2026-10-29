// Package media_management coordinates independent media API operations.
// It owns neither their resource state nor a cross-item transaction.
package media_management

import (
	"context"
	"errors"
	contract "media_agent/media_worker/biz/shared/media_management"
	"sync"
	"time"
)

const MaxItems = 20

var ErrInvalidBatch = errors.New("invalid media batch")

type Item[P any] struct {
	ID     string
	Params P
}
type Result[R any] struct {
	ID, Status string
	Data       R
	Failure    *Failure
}
type Summary struct{ Total, Succeeded, Failed, Unknown, NotDispatched int }
type BatchResult[R any] struct {
	Results []Result[R]
	Summary Summary
}

// Failure classifies the actual API outcome. Unknown is reserved for a write
// that may have committed; Stop prevents new dispatch after authority failure.
type Failure = contract.Failure

type Batch[P, R any] struct {
	ReadOnly     bool
	Validate     func(P) error
	DuplicateKey func(P) string
	Execute      func(context.Context, P) (R, error)
	// DeadlineReserve leaves time for the transport to serialize partial results.
	DeadlineReserve time.Duration
}

func (b Batch[P, R]) Run(ctx context.Context, items []Item[P]) (BatchResult[R], error) {
	result := BatchResult[R]{}
	if len(items) == 0 || len(items) > MaxItems || b.Execute == nil {
		return result, ErrInvalidBatch
	}
	ids, keys := map[string]bool{}, map[string]bool{}
	for _, item := range items {
		if len(item.ID) == 0 || len(item.ID) > 64 || ids[item.ID] {
			return result, ErrInvalidBatch
		}
		ids[item.ID] = true
		if b.Validate != nil {
			if err := b.Validate(item.Params); err != nil {
				return result, err
			}
		}
		if b.DuplicateKey != nil {
			key := b.DuplicateKey(item.Params)
			if key != "" {
				if keys[key] {
					return result, ErrInvalidBatch
				}
				keys[key] = true
			}
		}
	}
	// Structure preflight completed before any executor can produce a side effect.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if deadline, ok := ctx.Deadline(); ok {
		reserve := b.DeadlineReserve
		if reserve == 0 {
			reserve = 2 * time.Second
		}
		var stop context.CancelFunc
		runCtx, stop = context.WithDeadline(runCtx, deadline.Add(-reserve))
		defer stop()
	}
	result.Results = make([]Result[R], len(items))
	for i, item := range items {
		result.Results[i] = Result[R]{ID: item.ID, Status: "not_dispatched", Failure: &Failure{Code: "not_dispatched", Message: "Batch stopped before this item was dispatched", NotDispatched: true}}
	}
	count := 1
	if b.ReadOnly {
		count = 4
	}
	count = min(count, len(items))
	var mu sync.Mutex
	next := 0
	stopped := false
	var wg sync.WaitGroup
	for n := 0; n < count; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				if stopped || runCtx.Err() != nil || next >= len(items) {
					mu.Unlock()
					return
				}
				i := next
				next++
				mu.Unlock()
				data, err := b.Execute(runCtx, items[i].Params)
				row := Result[R]{ID: items[i].ID, Status: "succeeded", Data: data}
				if err != nil {
					var failure *Failure
					if !errors.As(err, &failure) {
						failure = &Failure{Code: "operation_failed", Message: "Media operation failed"}
					}
					copy := *failure
					row.Failure = &copy
					row.Status = "failed"
					if copy.Unknown {
						row.Status = "outcome_unknown"
					} else if copy.NotDispatched {
						row.Status = "not_dispatched"
					}
					if copy.Stop {
						mu.Lock()
						stopped = true
						mu.Unlock()
					}
				}
				result.Results[i] = row
			}
		}()
	}
	wg.Wait()
	result.Summary.Total = len(items)
	for _, row := range result.Results {
		switch row.Status {
		case "succeeded":
			result.Summary.Succeeded++
		case "failed":
			result.Summary.Failed++
		case "outcome_unknown":
			result.Summary.Unknown++
		case "not_dispatched":
			result.Summary.NotDispatched++
		}
	}
	return result, nil
}

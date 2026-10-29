package media_management

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPreflightNeverPartiallyCreates(t *testing.T) {
	calls := 0
	b := Batch[int, int]{Validate: func(v int) error {
		if v < 0 {
			return ErrInvalidBatch
		}
		return nil
	}, DuplicateKey: func(v int) string { return "same" }, Execute: func(context.Context, int) (int, error) { calls++; return 1, nil }}
	for _, items := range [][]Item[int]{{{ID: "a", Params: 1}, {ID: "b", Params: -1}}, {{ID: "a", Params: 1}, {ID: "a", Params: 2}}, {{ID: "a", Params: 1}, {ID: "b", Params: 1}}} {
		if _, err := b.Run(context.Background(), items); err == nil {
			t.Fatal("invalid batch accepted")
		}
	}
	if calls != 0 {
		t.Fatal("side effect before whole-batch preflight")
	}
}
func TestPartialWritesAreNotReplayedOrRolledBack(t *testing.T) {
	var calls []int
	b := Batch[int, int]{Execute: func(_ context.Context, v int) (int, error) {
		calls = append(calls, v)
		if v == 2 {
			return 0, &Failure{Code: "version_conflict", Message: "changed"}
		}
		if v == 3 {
			return 0, &Failure{Code: "outcome_unknown", Message: "response lost", Unknown: true}
		}
		return v * 10, nil
	}}
	result, err := b.Run(context.Background(), []Item[int]{{"a", 1}, {"b", 2}, {"c", 3}, {"d", 4}})
	if err != nil || len(calls) != 4 || result.Summary.Succeeded != 2 || result.Summary.Unknown != 1 || result.Results[3].Data != 40 {
		t.Fatalf("wrong partial results: %+v %v", result, err)
	}
	for i, v := range calls {
		if v != i+1 {
			t.Fatal("writes out of order")
		}
	}
}
func TestAuthorityFailureStopsLaterWrites(t *testing.T) {
	b := Batch[int, int]{Execute: func(_ context.Context, v int) (int, error) {
		if v == 2 {
			return 0, &Failure{Code: "authorization_unavailable", Message: "unavailable", Stop: true, NotDispatched: true}
		}
		return v, nil
	}}
	result, err := b.Run(context.Background(), []Item[int]{{"a", 1}, {"b", 2}, {"c", 3}})
	if err != nil || result.Summary.Succeeded != 1 || result.Summary.NotDispatched != 2 {
		t.Fatalf("lost accepted write or sent after authority failure: %+v", result)
	}
}
func TestReadsBoundConcurrencyAndPreserveInputOrder(t *testing.T) {
	var running, maxRunning atomic.Int32
	gate := make(chan struct{})
	started := make(chan struct{}, 4)
	b := Batch[int, int]{ReadOnly: true, Execute: func(_ context.Context, v int) (int, error) {
		n := running.Add(1)
		for old := maxRunning.Load(); n > old; old = maxRunning.Load() {
			if maxRunning.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		<-gate
		running.Add(-1)
		return v, nil
	}}
	done := make(chan BatchResult[int], 1)
	go func() {
		r, _ := b.Run(context.Background(), []Item[int]{{"a", 1}, {"b", 2}, {"c", 3}, {"d", 4}})
		done <- r
	}()
	for i := 0; i < 4; i++ {
		<-started
	}
	close(gate)
	r := <-done
	if maxRunning.Load() != 4 {
		t.Fatal("read concurrency not bounded to four")
	}
	for i, row := range r.Results {
		if row.Data != i+1 {
			t.Fatal("result order changed")
		}
	}
}
func TestExpiredBatchNeverDispatches(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	b := Batch[int, int]{Execute: func(context.Context, int) (int, error) {
		t.Fatal("expired item dispatched")
		return 0, errors.New("unexpected")
	}}
	r, err := b.Run(ctx, []Item[int]{{"a", 1}})
	if err != nil || r.Summary.NotDispatched != 1 {
		t.Fatalf("wrong expiry result %+v %v", r, err)
	}
}

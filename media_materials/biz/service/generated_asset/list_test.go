package generated_asset

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	domain "media_agent/media_materials/biz/domain/generated_asset"
	"media_agent/media_materials/biz/shared/objectstorage"
)

type listRepo struct {
	importRepo
	items []*domain.GeneratedAsset
}

func (r *listRepo) List(context.Context, domain.ListQuery) ([]*domain.GeneratedAsset, int, error) {
	return r.items, 24, nil
}

type blockedStorage struct {
	started      chan struct{}
	release      chan struct{}
	active, peak atomic.Int32
	err          error
}

func (s *blockedStorage) Stat(context.Context, string) (objectstorage.ObjectInfo, error) {
	return objectstorage.ObjectInfo{}, errors.New("unexpected HEAD")
}
func (s *blockedStorage) PresignGet(ctx context.Context, p string) (objectstorage.ResolvedObject, error) {
	n := s.active.Add(1)
	defer s.active.Add(-1)
	for old := s.peak.Load(); n > old; old = s.peak.Load() {
		if s.peak.CompareAndSwap(old, n) {
			break
		}
	}
	s.started <- struct{}{}
	select {
	case <-ctx.Done():
		return objectstorage.ResolvedObject{}, ctx.Err()
	case <-s.release:
	}
	if err := ctx.Err(); err != nil {
		return objectstorage.ResolvedObject{}, err
	}
	if s.err != nil {
		return objectstorage.ResolvedObject{}, s.err
	}
	return objectstorage.ResolvedObject{URL: "https://storage.example/" + p, ExpiresAt: time.Now().Add(time.Minute).Unix()}, nil
}

func TestListStorageIsBoundedParallelOrderedAndFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name         string
		fail, cancel bool
	}{{"ordered", false, false}, {"failure", true, false}, {"cancel", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			r := &listRepo{}
			for i := range 24 {
				r.items = append(r.items, &domain.GeneratedAsset{ID: fmt.Sprint(i), ObjectPath: fmt.Sprint(i)})
			}
			storage := &blockedStorage{started: make(chan struct{}, 24), release: make(chan struct{})}
			if tc.fail {
				storage.err = errors.New("unavailable")
			}
			s := NewService(r, importProduct{}, importVIP{})
			s.ConfigureHistoryImport(storage, importKinds{})
			done := make(chan struct{})
			var items []*domain.GeneratedAsset
			var total int
			var err error
			go func() { defer close(done); items, total, err = s.List(ctx, ListQuery{Limit: 24}) }()
			// No timing-based speed assertion: eight independent resolutions must
			// enter while all remain blocked, and the page never exceeds that cap.
			for range 8 {
				select {
				case <-storage.started:
				case <-ctx.Done():
					t.Fatal("storage requests did not overlap")
				}
			}
			if tc.cancel {
				cancel()
			}
			close(storage.release)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("list did not finish")
			}
			if storage.peak.Load() != 8 || storage.active.Load() != 0 {
				t.Fatal("unbounded fan-out or leaked operation")
			}
			if tc.fail || tc.cancel {
				if err == nil || items != nil || total != 0 {
					t.Fatal("returned partial success")
				}
				return
			}
			if err != nil || total != 24 || len(items) != 24 {
				t.Fatal("page contract changed")
			}
			for i, item := range items {
				if item.ID != fmt.Sprint(i) || item.URL != "https://storage.example/"+fmt.Sprint(i) {
					t.Fatal("page reordered or URL missing")
				}
			}
		})
	}
}

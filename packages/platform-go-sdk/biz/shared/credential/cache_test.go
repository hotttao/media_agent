package credential

import (
	"context"
	"errors"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentExchangeAndIndependentCancellation(t *testing.T) {
	cache := NewCache()
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	exchange := func(context.Context) (contract.Token, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return contract.Token{AccessToken: "mac_" + strings.Repeat("a", 64), ExpiresIn: 3600}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := cache.Get(ctx, "a", exchange); first <- err }()
	<-started
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := cache.Get(context.Background(), "a", exchange)
			if err != nil || v == "" {
				t.Errorf("token: %v", err)
			}
		}()
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("exchanges=%d", calls.Load())
	}
	cache.Invalidate("a")
	if _, err := cache.Get(context.Background(), "a", exchange); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("invalidation did not refresh")
	}
}
func TestCacheExpiryIsolationAndBadResponse(t *testing.T) {
	cache := NewCache()
	var calls int
	exchange := func(context.Context) (contract.Token, error) {
		calls++
		return contract.Token{AccessToken: "mac_" + strings.Repeat("a", 64), ExpiresIn: 3600}, nil
	}
	for _, key := range []string{"one", "two", "one"} {
		if _, err := cache.Get(context.Background(), key, exchange); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	cache.mu.Lock()
	cache.tokens["one"] = tokenEntry{"old", time.Now().Add(-time.Second)}
	cache.mu.Unlock()
	_, _ = cache.Get(context.Background(), "one", exchange)
	if calls != 3 {
		t.Fatal(calls)
	}
	for _, v := range []contract.Token{{AccessToken: "secret", ExpiresIn: 3600}, {AccessToken: "mac_" + strings.Repeat("a", 64), ExpiresIn: 10}} {
		_, err := cache.Get(context.Background(), "invalid", func(context.Context) (contract.Token, error) { return v, nil })
		if err == nil {
			t.Fatal("accepted invalid token")
		}
	}
}

// Package credential provides instance-local, coalesced application token caching.
package credential

import (
	"context"
	"encoding/hex"
	"errors"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	"strings"
	"sync"
	"time"
)

type tokenEntry struct {
	token   string
	expires time.Time
}
type pendingToken struct {
	done  chan struct{}
	value string
	err   error
}
type Cache struct {
	mu      sync.Mutex
	tokens  map[string]tokenEntry
	pending map[string]*pendingToken
}

func NewCache() *Cache {
	return &Cache{tokens: map[string]tokenEntry{}, pending: map[string]*pendingToken{}}
}

func (c *Cache) Invalidate(key string) { c.mu.Lock(); defer c.mu.Unlock(); delete(c.tokens, key) }
func (c *Cache) Get(ctx context.Context, key string, exchange func(context.Context) (contract.Token, error)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c.mu.Lock()
	if v, ok := c.tokens[key]; ok && time.Now().Before(v.expires) {
		c.mu.Unlock()
		return v.token, nil
	}
	p, ok := c.pending[key]
	if !ok {
		p = &pendingToken{done: make(chan struct{})}
		c.pending[key] = p
		go func() {
			started := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			v, e := exchange(ctx)
			if e == nil {
				raw, err := hex.DecodeString(strings.TrimPrefix(v.AccessToken, "mac_"))
				if err != nil || !strings.HasPrefix(v.AccessToken, "mac_") || len(raw) != 32 || v.ExpiresIn <= 30 {
					e = errors.New("invalid platform token response")
				}
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if e == nil {
				if len(c.tokens) >= 1000 {
					for k := range c.tokens {
						delete(c.tokens, k)
						break
					}
				}
				ttl := v.ExpiresIn
				if ttl > 3600 {
					ttl = 3600
				}
				c.tokens[key] = tokenEntry{v.AccessToken, started.Add(time.Duration(ttl-30) * time.Second)}
			}
			p.value, p.err = v.AccessToken, e
			delete(c.pending, key)
			close(p.done)
		}()
	}
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-p.done:
		return p.value, p.err
	}
}

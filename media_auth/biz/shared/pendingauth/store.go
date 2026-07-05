// Package pendingauth 是 MCP OAuth 授权流程的内存态 PKCE state 存储（10min TTL）。
//
// 不落库：state → PendingAuth 在 StartAuthorization 写入、HandleCallback 消费（一次性）。
// 后台 goroutine 每分钟清过期条目。对齐 control-plane services/mcp-oauth.ts pendingAuths。
package pendingauth

import (
	"sync"
	"time"
)

// PendingAuth 是一次待完成的 MCP OAuth 授权流程状态。
type PendingAuth struct {
	UserID       string
	ServerOrigin string
	CodeVerifier string
	WorkspaceID  string
	CreatedAt    time.Time
}

// Store 是 PendingAuth 的内存存储（线程安全）。
type Store struct {
	mu   sync.Mutex
	data map[string]PendingAuth
	ttl  time.Duration
}

// NewStore 构造 Store 并启动过期清理 goroutine。ttl 默认 10min。
func NewStore() *Store {
	s := &Store{data: make(map[string]PendingAuth), ttl: 10 * time.Minute}
	go s.cleanupLoop()
	return s
}

// Put 写入 state → PendingAuth。
func (s *Store) Put(state string, p PendingAuth) {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	s.mu.Lock()
	s.data[state] = p
	s.mu.Unlock()
}

// Consume 取出并删除（一次性）。不存在返回 (PendingAuth{}, false)。
func (s *Store) Consume(state string) (PendingAuth, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data[state]
	if ok {
		delete(s.data, state)
	}
	return p, ok
}

// cleanupLoop 每分钟删过期条目（> ttl）。
func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-s.ttl)
		s.mu.Lock()
		for k, v := range s.data {
			if v.CreatedAt.Before(cutoff) {
				delete(s.data, k)
			}
		}
		s.mu.Unlock()
	}
}

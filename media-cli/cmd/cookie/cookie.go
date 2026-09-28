// Package cookie 提供 media-cli 跨命令复用的持久化 cookie store + client middleware。
//
// 背景：media-cli 测 session-based 接口（如 media_auth 的 auth：login 下发 cookie，
// 后续 me/change-password 依赖 cookie）。media-cli 每次命令是独立进程，进程内内存在
// 命令间丢失，故 cookie 需持久化落盘跨命令复用——login 一次，后续命令直接带 cookie。
//
// 隔离：按 hostname 区分（HTTP Cookie 不按端口隔离）。落盘 media-cli/.cookies/<hostname>/cookies.json，
// 文件权限 0600，不进版本库（.gitignore 忽略 .cookies/）。
//
// middleware：注入 typed client（WithHertzClientMiddleware），每请求从 store 取该 host cookie
// 塞 req.Cookie；响应阶段读 Set-Cookie 更新 store + 落盘。logout 显式清该 host store。
package cookie

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
)

// cookieEntry 是持久化的单条 cookie（对齐 net/http.Cookie 关键字段）。
type cookieEntry struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	MaxAge   int    `json:"max_age,omitempty"`
	HTTPOnly bool   `json:"http_only,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
}

// Store 是按 hostname 隔离的持久化 cookie 存储。
type Store struct {
	mu  sync.Mutex
	dir string // .cookies 根目录
}

// New 构造 Store，cookie 落盘到 rootDir/.cookies/<hostname>/cookies.json。
func New(rootDir string) *Store {
	return &Store{dir: filepath.Join(rootDir, ".cookies")}
}

// Middleware 返回 client.Middleware：注入 cookie（从 store 读）+ 收集响应 Set-Cookie（写 store）。
// host 是被测服务地址（用于 store 隔离 + 落盘路径）。
func (s *Store) Middleware(host string) client.Middleware {
	return func(next client.Endpoint) client.Endpoint {
		return func(ctx context.Context, req *protocol.Request, resp *protocol.Response) (err error) {
			// Explicit API credentials take precedence over persisted browser state.
			if token := os.Getenv("MEDIA_API_TOKEN"); token != "" {
				req.Header.Del("Cookie")
				req.Header.Set("Authorization", "Bearer "+token)
				return next(ctx, req, resp)
			}
			// 请求前：从 store 取该 host cookie 注入 req。
			s.inject(host, req)
			// 执行。
			err = next(ctx, req, resp)
			if err != nil {
				return err
			}
			// 响应后：读 Set-Cookie 更新 store + 落盘。
			s.collect(host, resp)
			return nil
		}
	}
}

// Clear 删除该 host 的所有 cookie（logout 用）。
func (s *Store) Clear(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.RemoveAll(s.hostDir(host))
}

// inject 从 store 读该 host cookie，塞进 req 的 Cookie 头。
func (s *Store) inject(host string, req *protocol.Request) {
	s.mu.Lock()
	entries := s.loadLocked(host)
	s.mu.Unlock()
	for _, c := range entries {
		req.SetCookie(c.Name, c.Value)
	}
}

// collect 读 resp 的 Set-Cookie，更新 store 并落盘。
func (s *Store) collect(host string, resp *protocol.Response) {
	var entries []cookieEntry
	resp.Header.VisitAllCookie(func(_, value []byte) {
		if c := parseSetCookie(string(value)); c != nil {
			entries = append(entries, *c)
		}
	})
	if len(entries) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 合并：已有 store 里未被本次响应覆盖的 cookie 保留。
	existing := s.loadLocked(host)
	merged := mergeCookies(existing, entries)
	s.saveLocked(host, merged)
}

// parseSetCookie 解析单个 Set-Cookie 值（如 "token=xxx; Path=/; HttpOnly"）。
func parseSetCookie(raw string) *cookieEntry {
	parts := strings.Split(raw, ";")
	if len(parts) == 0 {
		return nil
	}
	kv := strings.SplitN(strings.TrimSpace(parts[0]), "=", 2)
	if len(kv) != 2 {
		return nil
	}
	c := &cookieEntry{Name: strings.TrimSpace(kv[0]), Value: strings.TrimSpace(kv[1])}
	for _, attr := range parts[1:] {
		attr = strings.TrimSpace(attr)
		switch {
		case strings.EqualFold(attr, "HttpOnly"):
			c.HTTPOnly = true
		case strings.EqualFold(attr, "Secure"):
			c.Secure = true
		case strings.HasPrefix(strings.ToLower(attr), "path="):
			c.Path = strings.TrimPrefix(attr, "Path=")
		case strings.HasPrefix(strings.ToLower(attr), "domain="):
			c.Domain = strings.TrimPrefix(attr, "Domain=")
		}
	}
	return c
}

// mergeCookies 合并：new 覆盖 existing 同名 cookie，其余保留。
func mergeCookies(existing, new []cookieEntry) []cookieEntry {
	byName := map[string]cookieEntry{}
	for _, c := range existing {
		byName[c.Name] = c
	}
	for _, c := range new {
		byName[c.Name] = c
	}
	out := make([]cookieEntry, 0, len(byName))
	for _, c := range byName {
		out = append(out, c)
	}
	return out
}

// hostDir 返回该服务地址所属 hostname 的 cookie 目录路径。
func (s *Store) hostDir(host string) string {
	// Cookie 的 host-only 作用域不包含端口：登录服务与其他服务只要位于同一
	// hostname，就必须跨进程复用同一份会话 Cookie。
	safe := ""
	if parsed, err := url.Parse(host); err == nil {
		safe = parsed.Hostname()
	}
	if safe == "" {
		safe = strings.NewReplacer("http://", "", "https://", "", ":", "_", "/", "").Replace(host)
	}
	return filepath.Join(s.dir, safe)
}

// loadLocked 从磁盘加载该 host 的 cookie（调用方持锁）。
func (s *Store) loadLocked(host string) []cookieEntry {
	data, err := os.ReadFile(filepath.Join(s.hostDir(host), "cookies.json"))
	if err != nil {
		return nil
	}
	var entries []cookieEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	return entries
}

// saveLocked 把 cookie 写盘（调用方持锁）。
func (s *Store) saveLocked(host string, entries []cookieEntry) {
	dir := s.hostDir(host)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "cookies.json"), data, 0o600)
}

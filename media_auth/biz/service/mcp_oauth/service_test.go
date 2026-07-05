// service_test.go：mcp_oauth 用例 service 单测（fake repo + httptest refresh endpoint）。
//
// 覆盖 GetValidAccessToken 刷新失败宽限算法：
//   - token 不存在 → ""
//   - token 未过期 → 直接返回 access
//   - 过期 + refresh 成功 → upsert + 返回新 access
//   - 过期 + permanent 失败 + 宽限窗口内 → ""
//   - 过期 + permanent 失败 + 超宽限 → 删 token + ErrReauthRequired
//   - 过期 + 暂态失败（5xx）→ ""
//
// refresh endpoint 用 httptest.Server 模拟；其余 repo 方法用 fake。
package mcpoauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
	mcpclient "media_agent/media_auth/biz/shared/client/mcp"
	"media_agent/media_auth/biz/shared/pendingauth"
)

// fakeRepo 实现 domainmcp.McpRepo（内存版，仅覆盖 GetValidAccessToken 所需方法）。
type fakeRepo struct {
	mu            sync.Mutex
	tokens        map[string]domainmcp.McpToken // key: userID+":"+origin
	clients       map[string]domainmcp.McpClient
	refreshCalled int
	upsertCalled  int
	deleteCalled  int
	// 控制 RecordRefreshFailure 返回值，模拟宽限窗口是否超期。
	failCount     int
	firstFailAged bool // true → firstFailAt = now - 700s（超宽限）
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{tokens: map[string]domainmcp.McpToken{}, clients: map[string]domainmcp.McpClient{}}
}

func (r *fakeRepo) UpsertClient(_ context.Context, c domainmcp.McpClient) error {
	r.clients[c.ServerOrigin] = c
	return nil
}
func (r *fakeRepo) GetClient(_ context.Context, origin string) (domainmcp.McpClient, error) {
	c, ok := r.clients[origin]
	if !ok {
		return domainmcp.McpClient{}, domainmcp.ErrClientNotFound
	}
	return c, nil
}
func (r *fakeRepo) UpsertToken(_ context.Context, userID, origin string, resp domainmcp.TokenResponse, ttl int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upsertCalled++
	r.tokens[userID+":"+origin] = domainmcp.McpToken{UserID: userID, ServerOrigin: origin, AccessToken: resp.AccessToken, RefreshToken: resp.RefreshToken}
	return nil
}
func (r *fakeRepo) GetToken(_ context.Context, userID, origin string) (domainmcp.McpToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[userID+":"+origin]
	if !ok {
		return domainmcp.McpToken{}, domainmcp.ErrTokenNotFound
	}
	return t, nil
}
func (r *fakeRepo) ListTokensForUser(context.Context, string) ([]domainmcp.McpToken, error) {
	return nil, nil
}
func (r *fakeRepo) RecordRefreshFailure(_ context.Context, userID, origin string) (int, *time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshCalled++
	r.failCount++
	var first *time.Time
	if r.firstFailAged {
		t := time.Now().Add(-700 * time.Second)
		first = &t
	} else {
		t := time.Now()
		first = &t
	}
	return r.failCount, first, nil
}
func (r *fakeRepo) DeleteToken(_ context.Context, userID, origin string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleteCalled++
	delete(r.tokens, userID+":"+origin)
	return nil
}

// ── 用例 ──────────────────────────────────────────────────────────────

func newSvc(repo *fakeRepo) *Service {
	return NewService(repo, mcpclient.NewClient(nil), pendingauth.NewStore())
}

func TestGetValidAccessToken_TokenNotFound(t *testing.T) {
	svc := newSvc(newFakeRepo())
	got, err := svc.GetValidAccessToken(context.Background(), "u1", "https://mcp.example.com", false)
	if err != nil || got != "" {
		t.Errorf("missing token: want ('',nil), got (%q,%v)", got, err)
	}
}

func TestGetValidAccessToken_NotExpired(t *testing.T) {
	repo := newFakeRepo()
	future := time.Now().Add(time.Hour)
	repo.tokens["u1:https://mcp.example.com"] = domainmcp.McpToken{
		UserID: "u1", ServerOrigin: "https://mcp.example.com",
		AccessToken: "valid-access", ExpiresAt: &future,
	}
	svc := newSvc(repo)
	got, err := svc.GetValidAccessToken(context.Background(), "u1", "https://mcp.example.com", false)
	if err != nil || got != "valid-access" {
		t.Errorf("not expired: want valid-access, got %q err=%v", got, err)
	}
}

func TestGetValidAccessToken_RefreshSuccess(t *testing.T) {
	// httptest refresh endpoint 返回新 access token。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(domainmcp.TokenResponse{AccessToken: "refreshed-access", RefreshToken: "rt2", ExpiresIn: 1800})
	}))
	defer srv.Close()

	repo := newFakeRepo()
	past := time.Now().Add(-time.Hour) // 已过期
	repo.tokens["u1:https://mcp.example.com"] = domainmcp.McpToken{
		UserID: "u1", ServerOrigin: "https://mcp.example.com",
		AccessToken: "old", RefreshToken: "rt1", ExpiresAt: &past,
	}
	repo.clients["https://mcp.example.com"] = domainmcp.McpClient{
		ServerOrigin: "https://mcp.example.com",
		Metadata:     domainmcp.Metadata{TokenEndpoint: srv.URL},
	}
	svc := newSvc(repo)

	got, err := svc.GetValidAccessToken(context.Background(), "u1", "https://mcp.example.com", false)
	if err != nil {
		t.Fatalf("refresh success: %v", err)
	}
	if got != "refreshed-access" {
		t.Errorf("want refreshed-access, got %q", got)
	}
	if repo.upsertCalled != 1 {
		t.Errorf("upsert should be called once, got %d", repo.upsertCalled)
	}
}

func TestGetValidAccessToken_PermanentFail_WithinGrace(t *testing.T) {
	// httptest 返回 400 invalid_grant（permanent），但宽限窗口内（firstFailAt=now）→ 返回 ""。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer srv.Close()

	repo := newFakeRepo()
	past := time.Now().Add(-time.Hour)
	repo.tokens["u1:https://mcp.example.com"] = domainmcp.McpToken{
		UserID: "u1", ServerOrigin: "https://mcp.example.com",
		AccessToken: "old", RefreshToken: "rt1", ExpiresAt: &past,
	}
	repo.clients["https://mcp.example.com"] = domainmcp.McpClient{
		ServerOrigin: "https://mcp.example.com",
		Metadata:     domainmcp.Metadata{TokenEndpoint: srv.URL},
	}
	repo.firstFailAged = false // 窗口内
	svc := newSvc(repo)

	got, err := svc.GetValidAccessToken(context.Background(), "u1", "https://mcp.example.com", false)
	if err != nil || got != "" {
		t.Errorf("within grace: want ('',nil), got (%q,%v)", got, err)
	}
	if repo.deleteCalled != 0 {
		t.Errorf("token should NOT be deleted within grace, deleteCalled=%d", repo.deleteCalled)
	}
}

func TestGetValidAccessToken_PermanentFail_DropAfterGrace(t *testing.T) {
	// httptest 返回 invalid_grant（permanent），宽限超期（firstFailAged=true → firstFailAt=now-700s）→ 删 token + ErrReauthRequired。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer srv.Close()

	repo := newFakeRepo()
	past := time.Now().Add(-time.Hour)
	repo.tokens["u1:https://mcp.example.com"] = domainmcp.McpToken{
		UserID: "u1", ServerOrigin: "https://mcp.example.com",
		AccessToken: "old", RefreshToken: "rt1", ExpiresAt: &past,
	}
	repo.clients["https://mcp.example.com"] = domainmcp.McpClient{
		ServerOrigin: "https://mcp.example.com",
		Metadata:     domainmcp.Metadata{TokenEndpoint: srv.URL},
	}
	repo.failCount = domainmcp.RefreshFailMinCount - 1 // 让本次 RecordRefreshFailure 后达到阈值
	repo.firstFailAged = true                          // firstFailAt 超宽限
	svc := newSvc(repo)

	_, err := svc.GetValidAccessToken(context.Background(), "u1", "https://mcp.example.com", false)
	if !errors.Is(err, ErrReauthRequired) {
		t.Errorf("past grace: want ErrReauthRequired, got %v", err)
	}
	if repo.deleteCalled != 1 {
		t.Errorf("token should be deleted past grace, deleteCalled=%d", repo.deleteCalled)
	}
}

func TestGetValidAccessToken_TransientFail(t *testing.T) {
	// httptest 返回 500（暂态）→ 返回 ""，不删 token，不计数。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	past := time.Now().Add(-time.Hour)
	repo.tokens["u1:https://mcp.example.com"] = domainmcp.McpToken{
		UserID: "u1", ServerOrigin: "https://mcp.example.com",
		AccessToken: "old", RefreshToken: "rt1", ExpiresAt: &past,
	}
	repo.clients["https://mcp.example.com"] = domainmcp.McpClient{
		ServerOrigin: "https://mcp.example.com",
		Metadata:     domainmcp.Metadata{TokenEndpoint: srv.URL},
	}
	svc := newSvc(repo)

	got, err := svc.GetValidAccessToken(context.Background(), "u1", "https://mcp.example.com", false)
	if err != nil || got != "" {
		t.Errorf("transient: want ('',nil), got (%q,%v)", got, err)
	}
	if repo.refreshCalled != 0 {
		t.Errorf("transient fail should NOT record refresh failure, refreshCalled=%d", repo.refreshCalled)
	}
	if repo.deleteCalled != 0 {
		t.Errorf("transient fail should NOT delete token, deleteCalled=%d", repo.deleteCalled)
	}
}

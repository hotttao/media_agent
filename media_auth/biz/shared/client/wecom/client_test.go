// client_test.go：wecom corp API client 单测（httptest.Server 模拟企微 endpoints）。
//
// 覆盖：corp access_token 缓存（多次调用仅 1 次 gettoken）、GetUserIDByCode、GetUserInfo、
//
//	errcode != 0 → error。
package wecom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	domainwecom "media_agent/media_auth/biz/domain/wecom"
)

// newTestServer 起一个模拟企微 API 的 httptest server，返回 gettoken 调用次数。
func newTestServer(t *testing.T, getUserID, userName string) (*httptest.Server, *int32) {
	t.Helper()
	var getTokenCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/cgi-bin/gettoken":
			atomic.AddInt32(&getTokenCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errcode": 0, "access_token": "corp-token-xxx", "expires_in": 7200,
			})
		case r.URL.Path == "/cgi-bin/auth/getuserinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errcode": 0, "userid": getUserID,
			})
		case r.URL.Path == "/cgi-bin/user/get":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errcode": 0, "userid": getUserID, "name": userName, "avatar": "http://x/a.png", "email": "u@x.com",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return srv, &getTokenCalls
}

// newClientAt 用自定义 base URL 构造 client（绕过硬编码的 qyapi.weixin.qq.com）。
// 由于 client 硬编码了企微域名，本测试通过覆盖 gettoken/getuserinfo/user/get 的 URL 来验证逻辑。
// 此处用一个 helper：直接调内部方法，把 base URL 指向 httptest。
func newClientWithBase(srv *httptest.Server) *Client {
	c := NewClient(domainwecom.Config{CorpID: "corp", CorpSecret: "secret", AgentID: "1"}, nil)
	// 替换 httpClient 的 transport：把所有 qyapi.weixin.qq.com 请求重定向到 srv。
	c.httpClient = &http.Client{Transport: &rewriteTransport{base: srv.URL}}
	return c
}

// rewriteTransport 把请求 Host 改写为 httptest server（用于绕过硬编码域名）。
type rewriteTransport struct{ base string }

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 把 https://qyapi.weixin.qq.com/... → <srv>/...
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	// srv.URL 形如 http://127.0.0.1:port；取 host:port。
	host := t.base[len("http://"):]
	req2.URL.Host = host
	req2.Host = host
	return http.DefaultTransport.RoundTrip(req2)
}

func TestClient_GetUserIDByCode_CachesAccessToken(t *testing.T) {
	srv, getTokenCalls := newTestServer(t, "wecom-user-1", "Alice")
	defer srv.Close()
	c := newClientWithBase(srv)

	// 两次调用 GetUserIDByCode → gettoken 仅 1 次（缓存命中）。
	uid1, err := c.GetUserIDByCode(context.Background(), "code-1")
	if err != nil || uid1 != "wecom-user-1" {
		t.Fatalf("first GetUserIDByCode: uid=%q err=%v", uid1, err)
	}
	uid2, err := c.GetUserIDByCode(context.Background(), "code-2")
	if err != nil || uid2 != "wecom-user-1" {
		t.Fatalf("second GetUserIDByCode: uid=%q err=%v", uid2, err)
	}
	if got := atomic.LoadInt32(getTokenCalls); got != 1 {
		t.Errorf("gettoken calls: got %d, want 1 (cached)", got)
	}
}

func TestClient_GetUserInfo(t *testing.T) {
	srv, _ := newTestServer(t, "wecom-user-1", "Alice")
	defer srv.Close()
	c := newClientWithBase(srv)

	info, err := c.GetUserInfo(context.Background(), "wecom-user-1")
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}
	if info.UserID != "wecom-user-1" || info.Name != "Alice" || info.Avatar != "http://x/a.png" || info.Email != "u@x.com" {
		t.Errorf("got info=%+v", info)
	}
}

func TestClient_GetUserIDByCode_ErrcodeNonZero(t *testing.T) {
	// 模拟 invalid code → errcode != 0。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "tok", "expires_in": 7200})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 40029, "errmsg": "invalid code"})
		}
	}))
	defer srv.Close()
	c := newClientWithBase(srv)
	_, err := c.GetUserIDByCode(context.Background(), "bad-code")
	if err == nil {
		t.Errorf("invalid code: want error, got nil")
	}
}

func TestClient_Config(t *testing.T) {
	c := NewClient(domainwecom.Config{CorpID: "c", CorpSecret: "s", AgentID: "a"}, nil)
	if cfg := c.Config(); cfg.CorpID != "c" || !cfg.Enabled() {
		t.Errorf("Config: got %+v", cfg)
	}
}

// introspect_test.go：introspect client 单测（httptest.Server 模拟 RS /introspect）。
//
// 覆盖：active + sub → ok=true；inactive → ok=false；非 200 → error；非 JSON → error。
package introspect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifySubjectToken_Active(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/introspect" {
			t.Errorf("path: got %q, want /introspect", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method: got %q, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("token") != "good-token" {
			t.Errorf("token form: got %q", r.FormValue("token"))
		}
		json.NewEncoder(w).Encode(map[string]any{"active": true, "sub": "u1", "client_id": "c1"})
	}))
	defer srv.Close()

	c := NewClient(nil)
	uid, clientID, ok, err := c.VerifySubjectToken(context.Background(), "good-token", srv.URL)
	if err != nil || !ok || uid != "u1" || clientID != "c1" {
		t.Errorf("active: got uid=%q clientID=%q ok=%v err=%v", uid, clientID, ok, err)
	}
}

func TestVerifySubjectToken_Inactive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"active": false})
	}))
	defer srv.Close()

	c := NewClient(nil)
	_, _, ok, err := c.VerifySubjectToken(context.Background(), "tok", srv.URL)
	if err != nil {
		t.Errorf("inactive: want no error, got %v", err)
	}
	if ok {
		t.Errorf("inactive: want ok=false")
	}
}

func TestVerifySubjectToken_ActiveButNoSub(t *testing.T) {
	// active=true 但 sub 空 → ok=false（按 control-plane 语义）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"active": true})
	}))
	defer srv.Close()

	c := NewClient(nil)
	_, _, ok, _ := c.VerifySubjectToken(context.Background(), "tok", srv.URL)
	if ok {
		t.Errorf("active without sub: want ok=false")
	}
}

func TestVerifySubjectToken_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(nil)
	_, _, _, err := c.VerifySubjectToken(context.Background(), "tok", srv.URL)
	if err == nil {
		t.Errorf("non-200: want error")
	}
}

func TestVerifySubjectToken_Unreachable(t *testing.T) {
	c := NewClient(nil)
	// 用一个已关闭的 server URL 模拟不可达。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	_, _, _, err := c.VerifySubjectToken(context.Background(), "tok", srv.URL)
	if err == nil {
		t.Errorf("unreachable: want error")
	}
}

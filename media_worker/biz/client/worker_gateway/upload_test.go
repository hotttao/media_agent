package worker_gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"media_agent/media_worker/biz/shared/worker_runtime"
)

func TestUploadFileConfirmsStorageWithoutForwardingToken(t *testing.T) {
	for _, mode := range []string{"success", "already", "precondition", "lost_response", "redirect", "unsafe_header"} {
		t.Run(mode, func(t *testing.T) {
			// Valid PNG signature; protocol test, not a decoder validity test.
			data := append([]byte{137, 80, 78, 71, 13, 10, 26, 10}, bytes.Repeat([]byte{0}, 32)...)
			filename := filepath.Join(t.TempDir(), "result.png")
			if err := os.WriteFile(filename, data, 0600); err != nil {
				t.Fatal(err)
			}
			stored, puts, prepares, redirects := mode == "already", 0, 0, 0
			var mu sync.Mutex
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				redirects++
				w.WriteHeader(200)
			}))
			defer target.Close()
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				puts++
				if r.Method != "PUT" || r.Header.Get("Traceparent") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("If-None-Match") != "*" {
					t.Error("storage received unsafe request")
				}
				got, _ := io.ReadAll(r.Body)
				if !bytes.Equal(got, data) || r.ContentLength != int64(len(data)) {
					t.Error("file bytes changed")
				}
				if mode == "redirect" {
					w.Header().Set("Location", target.URL)
					w.WriteHeader(307)
					return
				}
				stored = true
				if mode == "lost_response" {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						conn.Close()
					}
					return
				}
				if mode == "precondition" {
					w.WriteHeader(412)
					return
				}
				w.WriteHeader(200)
			}))
			defer storage.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				prepares++
				if r.URL.Path != "/api/worker/v1/tasks/dispatch/uploads" || r.Header.Get("Traceparent") != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00" || r.Header.Get("Authorization") != "Bearer test-worker-token" {
					t.Error("gateway identity missing")
				}
				var in map[string]any
				if json.NewDecoder(r.Body).Decode(&in) != nil || in["content_type"] != "image/png" || in["attempt"] != float64(2) || in["candidate_index"] != float64(1) || len(in["checksum_sha256"].(string)) != 64 {
					t.Error("invalid manifest")
				}
				grant := uploadGrant{ObjectPath: "materials/org/scope/worker_generated/task/file", AlreadyUploaded: stored, UploadURL: storage.URL + "/object?private=test-signature", ExpiresAt: time.Now().Add(time.Minute).Unix(), Headers: map[string]string{"Content-Type": "image/png", "Content-MD5": in["content_md5"].(string), "If-None-Match": "*"}}
				if mode == "unsafe_header" {
					grant.Headers["Authorization"] = "forbidden"
				}
				json.NewEncoder(w).Encode(grant)
			}))
			defer api.Close()
			path, err := New(api.URL, "test-worker-token").UploadFile(context.Background(), "desktop", worker_runtime.Task{DispatchID: "dispatch", Attempt: 2, TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"}, 1, filename)
			mu.Lock()
			defer mu.Unlock()
			if mode == "redirect" || mode == "unsafe_header" {
				if err == nil || redirects != 0 || strings.Contains(err.Error(), "test-signature") {
					t.Fatal("unsafe upload accepted or secret exposed")
				}
				if mode == "unsafe_header" && puts != 0 {
					t.Fatal("unsafe header reached storage")
				}
			} else if err != nil || path == "" || !stored || (mode != "already" && (prepares != 2 || puts != 1)) || (mode == "already" && puts != 0) {
				t.Fatal("upload was not confirmed", err)
			}
		})
	}
}

func TestUploadFileRejectsNonMediaWithoutGatewayRequest(t *testing.T) {
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer api.Close()
	filename := filepath.Join(t.TempDir(), "not-an-image.png")
	os.WriteFile(filename, []byte("plain text, despite extension"), 0600)
	_, err := New(api.URL, "test").UploadFile(context.Background(), "worker", worker_runtime.Task{}, 0, filename)
	if err == nil || requests != 0 {
		t.Fatal("non-media requested upload grant")
	}
}

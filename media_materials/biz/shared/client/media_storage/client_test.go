package media_storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPutSendsInternalCredentialAndRawObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Query().Get("object_path") != "materials/org/file.png" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			return
		}
		if r.Header.Get("X-Media-Storage-Internal-Token") != "secret" || r.Header.Get("Content-Type") != "image/png" {
			t.Errorf("unexpected headers: %+v", r.Header)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "png" {
			t.Errorf("body=%q", body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object_path":"materials/org/file.png","size_bytes":3,"content_type":"image/png","checksum":"sha256:test"}`)
	}))
	defer server.Close()

	got, err := New(server.URL, "secret", time.Second).Put(context.Background(), PutInput{
		ObjectPath: "materials/org/file.png", ContentType: "image/png", SizeBytes: 3, Body: strings.NewReader("png"),
	})
	if err != nil || got.Checksum != "sha256:test" {
		t.Fatalf("Put=%+v, %v", got, err)
	}
}

func TestPutMapsConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusConflict) }))
	defer server.Close()
	_, err := New(server.URL, "secret", time.Second).Put(context.Background(), PutInput{Body: strings.NewReader("x")})
	if err != ErrConflict {
		t.Fatalf("error=%v, want ErrConflict", err)
	}
}

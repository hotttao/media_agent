package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuditSourceIDOnlyAcceptsMediaAIProvenance(t *testing.T) {
	for _, test := range []struct {
		name, metadata, want string
	}{
		{"object", `{"migration":{"source":"media_ai","source_id":"old-1"}}`, "old-1"},
		{"encoded", `"{\"migration\":{\"source\":\"media_ai\",\"source_id\":\"old-2\"}}"`, "old-2"},
		{"other source", `{"migration":{"source":"other","source_id":"old-3"}}`, ""},
		{"invalid", `not json`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := auditSourceID([]byte(test.metadata)); got != test.want {
				t.Fatalf("source ID = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMovementAuditAcceptsExactTextOnlySource(t *testing.T) {
	if !movementAuditReady("", "turn left", auditTarget{Content: "turn left"}) {
		t.Fatal("text-only Movement was rejected")
	}
	if movementAuditReady("", "turn left", auditTarget{Content: "turn right"}) ||
		movementAuditReady("", "turn left", auditTarget{Content: "turn left", ObjectPath: "unexpected"}) ||
		movementAuditReady("/uploads/move.mp4", "turn left", auditTarget{Content: "turn left"}) {
		t.Fatal("movement mapping accepted mismatched facts")
	}
}

func TestAuditMediaProbeIsReadOnlyAndRejectsBadResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Range") != "bytes=0-0" {
			t.Errorf("unexpected probe %s range=%q", r.Method, r.Header.Get("Range"))
		}
		switch r.URL.Path {
		case "/uploads/ok.png":
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte{0x89})
		case "/uploads/missing.png":
			w.WriteHeader(http.StatusNotFound)
		case "/uploads/html.png":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		case "/uploads/empty.png":
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusPartialContent)
		case "/uploads/redirect.png":
			http.Redirect(w, r, "/uploads/ok.png", http.StatusFound)
		}
	}))
	defer server.Close()
	client := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, test := range []struct{ path, want string }{
		{"/uploads/ok.png", ""},
		{"/uploads/missing.png", "HTTP 404"},
		{"/uploads/html.png", "HTML instead of media"},
		{"/uploads/empty.png", "empty or unreadable body"},
		{"/uploads/redirect.png", "HTTP 302"},
		{"http://other.example/uploads/ok.png", "invalid or foreign URL"},
		{"/private/file.png", "invalid or foreign URL"},
		{"", "missing URL"},
	} {
		if got := probeOneAuditMedia(context.Background(), client, server.URL, test.path); got != test.want {
			t.Fatalf("probe %q = %q, want %q", test.path, got, test.want)
		}
	}
	if got := probeOneAuditMedia(context.Background(), client, server.URL, "/uploads/../private/file.png"); !strings.Contains(got, "invalid") {
		t.Fatalf("path traversal was not rejected: %q", got)
	}
}

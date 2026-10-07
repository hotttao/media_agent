package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"media_agent/media_worker/internal/gateway"
	"media_agent/media_worker/internal/results"
)

func TestPendingResultRealProcessRestart(t *testing.T) {
	if os.Getenv("MEDIA_WORKER_TEST_SAVE_EXIT") == "1" {
		store, err := results.Open(os.Getenv("MEDIA_WORKER_TEST_STATE_DIR"), "http://test-gateway", "worker", "test-token")
		if err != nil {
			os.Exit(71)
		}
		err = store.Save(results.Record{Task: gateway.Task{DispatchID: "dispatch", Attempt: 1}, Key: "stable-complete", Output: map[string]any{"candidates": []any{map[string]any{"content": "saved result"}}}})
		if err != nil {
			os.Exit(72)
		}
		os.Exit(0)
	}
	dir := filepath.Join(t.TempDir(), "private")
	process := exec.Command(os.Args[0], "-test.run=^TestPendingResultRealProcessRestart$")
	process.Env = append(os.Environ(), "MEDIA_WORKER_TEST_SAVE_EXIT=1", "MEDIA_WORKER_TEST_STATE_DIR="+dir)
	if err := process.Run(); err != nil {
		t.Fatal("result persistence child failed")
	}
	store, err := results.Open(dir, "http://test-gateway", "worker", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err := store.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatal("child result not recovered")
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["idempotency_key"] != "stable-complete" {
			t.Error("replay changed identity")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"receipt_id":"confirmed","status":"submitted"}`))
	}))
	defer server.Close()
	// This recovery path has no executor argument: it cannot generate again.
	if !deliverStoredResult(context.Background(), gateway.New(server.URL, "test-token"), "worker", store, pending[0]) {
		t.Fatal("replay not acknowledged")
	}
	remaining, err := store.Pending()
	if err != nil || len(remaining) != 0 || calls != 1 {
		t.Fatal("confirmed pending result not retired")
	}
}

func TestSelectCapabilities(t *testing.T) {
	available := []gateway.Capability{
		{Key: "media.image.model", Version: 1, Concurrency: 1},
		{Key: "media.video.generate", Version: 1, Concurrency: 2},
	}
	selected, err := selectCapabilities(available, "media.video.generate@1,media.video.generate@1")
	if err != nil {
		t.Fatalf("selectCapabilities() error = %v", err)
	}
	if len(selected) != 1 || selected[0].Key != "media.video.generate" || selected[0].Concurrency != 2 {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestSelectCapabilitiesRejectsUnknownVersion(t *testing.T) {
	_, err := selectCapabilities([]gateway.Capability{{Key: "media.image.model", Version: 1}}, "media.image.model@2")
	if err == nil {
		t.Fatal("selectCapabilities() should reject an unsupported capability version")
	}
}

func TestParseMultipleFixtureResultsDoesNotExposeConfiguration(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `["https://storage.example/a"]`, `[42,true]`, `secret-invalid-json`} {
		if _, err := parseFixtureResultURLs(raw); err == nil || err.Error() == raw {
			t.Fatal("malformed candidate configuration accepted or leaked")
		}
	}
	urls, err := parseFixtureResultURLs(`["https://storage.example/a","https://storage.example/b"]`)
	if err != nil || len(urls) != 2 {
		t.Fatal("valid configuration rejected", err)
	}
}

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Protocol-only provider: persistent receipt semantics are covered separately
// with Materials/PostgreSQL. This verifies the actual Workflow HTTP client.
func TestMaterialsRegistrationExplicitReplayAfterTCPResponseLoss(t *testing.T) {
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "isolated-client-token")
	var calls atomic.Int64
	var invalid atomic.Bool
	key := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input UpsertGenAssetReq
		if r.Method != "POST" || r.URL.Path != "/api/internal/v1/generated-assets:upsert" || r.Header.Get("X-Media-Materials-Internal-Token") != "isolated-client-token" || json.NewDecoder(r.Body).Decode(&input) != nil || input.ResultKey != key || input.StepRunID != "step" {
			invalid.Store(true)
			w.WriteHeader(400)
			return
		}
		if calls.Add(1) == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				invalid.Store(true)
				return
			}
			_ = connection.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"accepted-asset"}`)
	}))
	defer server.Close()
	client := NewMaterialsClient(server.URL)
	defer client.http.CloseIdleConnections()
	request := &UpsertGenAssetReq{ResultKey: key, StepRunID: "step"}
	if id, err := client.UpsertGeneratedAsset(context.Background(), request); err == nil || id != "" || strings.Contains(err.Error(), server.URL) {
		t.Fatal("lost response exposed a result or transport details")
	}
	if calls.Load() != 1 {
		t.Fatal("client automatically resent ambiguous registration")
	}
	id, err := client.UpsertGeneratedAsset(context.Background(), request)
	if err != nil || id != "accepted-asset" || calls.Load() != 2 || invalid.Load() || request.ResultKey != key {
		t.Fatal("explicit replay lost generation identity")
	}
}

func TestMaterialsRegistrationRejectsInvalidSuccessAndHidesProviderBody(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"missing identity", 200, `{}`},
		{"blank identity", 200, `{"id":"  "}`},
		{"malformed", 200, `private-upstream-payload`},
		{"conflict", 409, `private-upstream-payload`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client := NewMaterialsClient(server.URL)
			defer client.http.CloseIdleConnections()
			id, err := client.UpsertGeneratedAsset(context.Background(), &UpsertGenAssetReq{})
			if err == nil || id != "" || strings.Contains(err.Error(), "private-upstream-payload") || strings.Contains(err.Error(), server.URL) {
				t.Fatal("invalid registration response accepted or leaked")
			}
		})
	}
}

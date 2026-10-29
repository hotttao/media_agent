package business_http

import (
	"context"
	"errors"
	"media_agent/hertz_infra/authn/mcpdelegation"
	contract "media_agent/media_worker/biz/shared/media_management"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriteAdmissionFailureStopsBatchWithoutRetry(t *testing.T) {
	for _, test := range []struct {
		status        int
		admission     bool
		unknown, stop bool
	}{{403, true, false, true}, {503, true, false, true}, {403, false, false, false}, {503, false, true, false}} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.Header.Get("Authorization") != "Bearer workflow-delegate" || r.Header.Get(mcpdelegation.ServiceHeader) != strings.Repeat("s", 32) {
				t.Error("wrong target credentials")
			}
			if test.admission {
				w.Header().Set("X-Media-MCP-Admission", "rejected")
			}
			w.WriteHeader(test.status)
		}))
		client, err := New(server.URL, strings.Repeat("s", 32), mcpdelegation.Workflow)
		if err != nil {
			t.Fatal(err)
		}
		ctx := contract.WithCredentials(context.Background(), contract.Credentials{Workflow: "workflow-delegate", Materials: "other-delegate"})
		_, err = client.Request(ctx, "POST", "/api/v1/jobs", nil, map[string]string{"idempotency_key": "one"})
		var failure *contract.Failure
		if !errors.As(err, &failure) || failure.Unknown != test.unknown || failure.Stop != test.stop || calls.Load() != 1 {
			t.Fatalf("wrong failure: %+v calls=%d", failure, calls.Load())
		}
		server.Close()
	}
}

func TestConflictReasonIsPreservedWithoutTrustingArbitraryUpstreamErrors(t *testing.T) {
	for _, code := range []string{"version_conflict", "idempotency_conflict", "untrusted detail"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Media-Workflow-Error", code)
			w.WriteHeader(409)
		}))
		client, err := New(server.URL, strings.Repeat("s", 32), mcpdelegation.Workflow)
		if err != nil {
			t.Fatal(err)
		}
		ctx := contract.WithCredentials(context.Background(), contract.Credentials{Workflow: "delegate"})
		_, err = client.Request(ctx, "POST", "/api/v1/jobs", nil, nil)
		var failure *contract.Failure
		expected := code
		if code == "untrusted detail" {
			expected = "conflict"
		}
		if !errors.As(err, &failure) || failure.Code != expected || failure.Unknown {
			t.Fatalf("wrong conflict classification: %+v", failure)
		}
		server.Close()
	}
}

func TestSingleRequestTimeoutDoesNotCancelIndependentBatchItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client, err := New(server.URL, strings.Repeat("s", 32), mcpdelegation.Workflow)
	if err != nil {
		t.Fatal(err)
	}
	client.timeout = 10 * time.Millisecond
	ctx := contract.WithCredentials(context.Background(), contract.Credentials{Workflow: "delegate"})
	_, err = client.Request(ctx, "POST", "/api/v1/jobs", nil, nil)
	var failure *contract.Failure
	if !errors.As(err, &failure) || !failure.Unknown || failure.Stop || ctx.Err() != nil {
		t.Fatalf("per-item timeout stopped the independent batch: %+v", failure)
	}
}

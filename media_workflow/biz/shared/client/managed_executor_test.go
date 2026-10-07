package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"go.opentelemetry.io/otel/propagation"
	"media_agent/hertz_infra/tracecontext"
)

func TestManagedExecutorSendsIdempotentInternalRequest(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/execute" || r.Method != "POST" {
			t.Error("wrong executor request")
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("managed executor carried user credential")
		}
		if r.Header.Get("Traceparent") != parent {
			t.Error("managed execution lost local correlation")
		}
		if r.Header.Get("Idempotency-Key") != "managed:dispatch-1:1" {
			t.Error("missing stable idempotency key")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"input":{"prompt":"test"}}` {
			t.Error("request body changed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":"result"}]}`))
	}))
	defer server.Close()
	httpClient, err := hclient.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	executor := &ManagedExecutor{Client: httpClient, BaseURL: server.URL}
	ctx := tracecontext.Extract(context.Background(), propagation.MapCarrier{"traceparent": parent})
	output, err := executor.Execute(ctx, []byte(`{"input":{"prompt":"test"}}`), "managed:dispatch-1:1")
	if err != nil || output["candidates"] == nil {
		t.Fatalf("executor output: %v", err)
	}
}

func TestManagedExecutorDoesNotExposeProviderResponseOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("provider-secret"))
	}))
	defer server.Close()
	httpClient, err := hclient.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&ManagedExecutor{Client: httpClient, BaseURL: server.URL}).Execute(context.Background(), []byte(`{}`), "idempotent")
	if err == nil || err.Error() != "managed executor status 502" {
		t.Fatalf("unexpected safe error: %v", err)
	}
}

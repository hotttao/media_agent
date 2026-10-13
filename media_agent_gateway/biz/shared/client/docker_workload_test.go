package client

import (
	"context"
	"errors"
	contract "media_agent/hertz_infra/agentworkload"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDockerWorkloadsRejectScalingAndPreserveRemote501(t *testing.T) {
	var calls atomic.Int32
	var status atomic.Int32
	status.Store(501)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/v1/workspaces/ws/container:ensure" || r.Header.Get("X-Runtime-Creation-Credential") != "fixture" {
			t.Errorf("wrong creation request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(`{"error":"not_implemented"}`))
	}))
	defer api.Close()
	d := DockerWorkloads{EnvironmentID: "env", BaseURL: api.URL, ServiceToken: "fixture"}
	s := contract.Spec{Driver: "docker", EnvironmentID: "env", WorkspaceID: "ws", Replicas: 1}
	for _, count := range []int32{0, 2} {
		s.Replicas = count
		if e := d.Save(context.Background(), &NativeWorkload{Spec: s, Version: 1}); !errors.Is(e, ErrWorkloadNotImplemented) {
			t.Fatalf("unsupported count %d: %v", count, e)
		}
	}
	s.Replicas = 1
	if e := d.Save(context.Background(), &NativeWorkload{Spec: s, Version: 1, Operation: "drain"}); !errors.Is(e, ErrWorkloadNotImplemented) {
		t.Fatalf("unsupported drain: %v", e)
	}
	if calls.Load() != 0 {
		t.Fatal("unsupported operation reached creation service")
	}
	if e := d.Save(context.Background(), &NativeWorkload{Spec: s, Version: 1}); !errors.Is(e, ErrWorkloadNotImplemented) {
		t.Fatalf("remote 501 was lost: %v", e)
	}
	if calls.Load() != 1 {
		t.Fatal("creation request count mismatch")
	}
	for _, test := range []struct {
		code int32
		want error
	}{{404, ErrWorkloadAbsent}, {409, ErrWorkloadConflict}, {501, ErrWorkloadNotImplemented}} {
		status.Store(test.code)
		if e := d.Save(context.Background(), &NativeWorkload{Spec: s, Version: 1}); !errors.Is(e, test.want) {
			t.Fatalf("remote status %d lost: %v", test.code, e)
		}
	}
}

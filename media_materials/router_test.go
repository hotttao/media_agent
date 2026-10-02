package main

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestGeneratedAssetRoutesAreRegisteredAndDeferredReachabilityIsUnavailable(t *testing.T) {
	h := server.Default()
	register(h)
	want := map[string]bool{
		"GET /api/v1/generated-assets":                  false,
		"GET /api/v1/generated-assets/:id":              false,
		"POST /api/internal/v1/generated-assets:upsert": false,
	}
	for _, route := range h.Engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, registered := range want {
		if !registered {
			t.Fatalf("route %s was not registered", route)
		}
	}
	response := ut.PerformRequest(h.Engine, "POST", "/api/internal/v1/reachability/asset", nil)
	if got := response.Result().StatusCode(); got != 404 {
		t.Fatalf("deferred reachability status = %d, want 404", got)
	}
}

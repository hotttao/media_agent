package main

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestDeferredRoutesAreUnavailableInOrganizationLibraryMilestone(t *testing.T) {
	h := server.Default()
	register(h)

	tests := []struct {
		method string
		path   string
	}{
		{method: "GET", path: "/api/v1/generated-assets"},
		{method: "POST", path: "/api/internal/v1/reachability/asset"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			response := ut.PerformRequest(h.Engine, tt.method, tt.path, nil)
			if got := response.Result().StatusCode(); got != 404 {
				t.Fatalf("status = %d, want 404", got)
			}
		})
	}
}

package job

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

// This covers the retirement extension and actual route wiring, not generated
// routing internals. The handler has no service injected: reaching it is a bug.
func TestLocalWorkerRoutesRetiredBeforeHandler(t *testing.T) {
	h := server.Default()
	Register(h)
	for _, path := range []string{"/api/v1/step-runs/claim", "/api/v1/step-runs/any/complete", "/api/v1/step-runs/any/fail"} {
		response := ut.PerformRequest(h.Engine, "POST", path, nil)
		if response.Result().StatusCode() != 410 {
			t.Fatalf("retired local worker route %s remained available", path)
		}
	}
}

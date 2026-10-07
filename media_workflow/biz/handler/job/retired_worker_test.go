package job

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

// Direct handlers must be safe even without the retirement middleware.
func TestLegacyWorkerHandlersCannotInvokeLocalExecution(t *testing.T) {
	h := server.New()
	h.POST("/claim", ClaimStepRun)
	h.POST("/complete", CompleteStepRun)
	h.POST("/fail", FailStepRun)
	for _, path := range []string{"/claim", "/complete", "/fail"} {
		response := ut.PerformRequest(h.Engine, "POST", path, nil)
		if response.Result().StatusCode() != 410 {
			t.Fatalf("legacy handler %s did not retire", path)
		}
	}
}

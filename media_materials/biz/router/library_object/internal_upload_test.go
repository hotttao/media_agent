package library_object

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestInternalUploadPreparationRequiresServiceCredential(t *testing.T) {
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "fixture-internal-secret")
	h := server.New()
	Register(h)
	for _, credential := range []string{"", "wrong"} {
		response := ut.PerformRequest(h.Engine, "POST", "/api/internal/v1/library-objects:prepareUpload", nil, ut.Header{Key: "X-Media-Materials-Internal-Token", Value: credential})
		if response.Result().StatusCode() != 401 {
			t.Fatal("internal upload preparation bypassed service credential")
		}
	}
}

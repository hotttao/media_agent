package generated_asset

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestInternalCredentialDoesNotBlockPublicGeneratedAssetReads(t *testing.T) {
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "internal-secret")
	h := server.Default()
	publicReached := false
	h.GET("/api/v1/generated-assets", append(rootMw(), func(context.Context, *app.RequestContext) { publicReached = true })...)
	h.POST("/api/internal/v1/generated-assets:upsert", append(_upsertgeneratedassetMw(), func(context.Context, *app.RequestContext) {})...)
	response := ut.PerformRequest(h.Engine, "GET", "/api/v1/generated-assets", nil)
	if response.Result().StatusCode() != 200 || !publicReached {
		t.Fatal("public generated asset read was blocked by internal middleware")
	}
	response = ut.PerformRequest(h.Engine, "POST", "/api/internal/v1/generated-assets:upsert", nil)
	if response.Result().StatusCode() != 401 {
		t.Fatal("internal generated asset write accepted without a service credential")
	}
}

func TestReusableAssetReadRequiresInternalServiceCredential(t *testing.T) {
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "internal-test-secret")
	h := server.Default()
	h.GET("/api/internal/v1/generated-assets/reusable/:id", append(_verifyreusablegeneratedassetMw(), func(_ context.Context, c *app.RequestContext) { c.Status(204) })...)
	for _, test := range []struct {
		credential string
		status     int
	}{{"", 401}, {"invalid", 401}, {"internal-test-secret", 204}} {
		response := ut.PerformRequest(h.Engine, "GET", "/api/internal/v1/generated-assets/reusable/gen_test", nil, ut.Header{Key: "X-Media-Materials-Internal-Token", Value: test.credential})
		if response.Result().StatusCode() != test.status {
			t.Fatal("internal reusable asset authorization failed")
		}
	}
}

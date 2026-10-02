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

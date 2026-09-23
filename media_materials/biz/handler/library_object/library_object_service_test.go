package library_object

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"media_agent/media_materials/biz/shared/auth"
	"media_agent/media_materials/biz/shared/objectstorage"
)

type fakeResolver struct {
	result objectstorage.ResolvedObject
	err    error
}

func (r fakeResolver) PresignGet(_ context.Context, _ string) (objectstorage.ResolvedObject, error) {
	return r.result, r.err
}

func TestResolveLibraryObjectAuthorizationAndErrors(t *testing.T) {
	previous := Resolver
	t.Cleanup(func() { Resolver = previous })

	h := server.Default()
	h.Use(auth.New(&auth.Config{DevUserID: "user-1", DevOrganizationID: "org-1"}).Handler())
	h.GET("/api/v1/library-objects:resolve", ResolveLibraryObject)

	t.Run("returns presigned URL for organization object", func(t *testing.T) {
		Resolver = fakeResolver{result: objectstorage.ResolvedObject{URL: "http://storage.test/media/object?signature=x", ExpiresAt: 42}}
		response := ut.PerformRequest(h.Engine, "GET", "/api/v1/library-objects:resolve?organization_id=org-1&object_path=materials/org-1/ai_clothing/material/image.png", nil)
		if got := response.Result().StatusCode(); got != 200 {
			t.Fatalf("status=%d body=%s", got, response.Result().Body())
		}
	})

	t.Run("hides unknown organization", func(t *testing.T) {
		response := ut.PerformRequest(h.Engine, "GET", "/api/v1/library-objects:resolve?organization_id=org-2&object_path=materials/org-2/ai_clothing/material/image.png", nil)
		if got := response.Result().StatusCode(); got != 404 {
			t.Fatalf("status=%d, want 404", got)
		}
	})

	t.Run("rejects path outside organization namespace", func(t *testing.T) {
		response := ut.PerformRequest(h.Engine, "GET", "/api/v1/library-objects:resolve?organization_id=org-1&object_path=materials/org-2/ai_clothing/material/image.png", nil)
		if got := response.Result().StatusCode(); got != 403 {
			t.Fatalf("status=%d, want 403", got)
		}
	})

	t.Run("maps provider not found", func(t *testing.T) {
		Resolver = fakeResolver{err: objectstorage.ErrNotFound}
		response := ut.PerformRequest(h.Engine, "GET", "/api/v1/library-objects:resolve?organization_id=org-1&object_path=materials/org-1/ai_clothing/material/missing.png", nil)
		if got := response.Result().StatusCode(); got != 404 {
			t.Fatalf("status=%d, want 404", got)
		}
	})

	t.Run("maps provider unavailable", func(t *testing.T) {
		Resolver = fakeResolver{err: errors.New("provider unavailable")}
		response := ut.PerformRequest(h.Engine, "GET", "/api/v1/library-objects:resolve?organization_id=org-1&object_path=materials/org-1/ai_clothing/material/image.png", nil)
		if got := response.Result().StatusCode(); got != 503 {
			t.Fatalf("status=%d, want 503", got)
		}
	})
}

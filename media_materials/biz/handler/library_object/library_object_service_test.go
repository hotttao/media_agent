package library_object

import (
	"context"
	"encoding/json"
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
		Resolver = fakeResolver{result: objectstorage.ResolvedObject{URL: "http://storage.test/media/object?signature=x", ExpiresAt: 42, ContentType: "image/png", SizeBytes: 100}}
		response := ut.PerformRequest(h.Engine, "GET", "/api/v1/library-objects:resolve?organization_id=org-1&object_path=materials/org-1/ai_clothing/material/image.png", nil)
		if got := response.Result().StatusCode(); got != 200 {
			t.Fatalf("status=%d body=%s", got, response.Result().Body())
		}
		var body struct {
			ContentType string `json:"content_type"`
			SizeBytes   int64  `json:"size_bytes"`
		}
		if json.Unmarshal(response.Result().Body(), &body) != nil || body.ContentType != "image/png" || body.SizeBytes != 100 {
			t.Fatal("stored media metadata missing from resolution response")
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

func TestResolveInternalLibraryObjectIsOrganizationScoped(t *testing.T) {
	previous := Resolver
	t.Cleanup(func() { Resolver = previous })
	Resolver = fakeResolver{result: objectstorage.ResolvedObject{URL: "http://storage.test/media/materials/org-1/image.png?fresh=1", ExpiresAt: 42, ContentType: "image/png", SizeBytes: 100}}
	h := server.Default()
	h.GET("/api/internal/v1/library-objects:resolve", ResolveInternalLibraryObject)
	for _, test := range []struct {
		path string
		want int
	}{
		{"materials/org-1/ai_clothing/material/image.png", 200},
		{"materials/org-2/ai_clothing/material/image.png", 403},
		{"materials/org-1/../org-2/image.png", 403},
	} {
		response := ut.PerformRequest(h.Engine, "GET", "/api/internal/v1/library-objects:resolve?organization_id=org-1&object_path="+test.path, nil)
		if got := response.Result().StatusCode(); got != test.want {
			t.Fatalf("path=%q status=%d want=%d", test.path, got, test.want)
		}
		if test.want == 200 {
			var body struct {
				ContentType string `json:"content_type"`
				SizeBytes   int64  `json:"size_bytes"`
			}
			if json.Unmarshal(response.Result().Body(), &body) != nil || body.ContentType != "image/png" || body.SizeBytes != 100 {
				t.Fatal("internal resolution omitted stored media metadata")
			}
		}
	}
}

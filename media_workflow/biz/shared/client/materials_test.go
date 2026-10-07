package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestResolveInternalObjectUsesServiceCredentialAndPath(t *testing.T) {
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "internal-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/internal/v1/library-objects:resolve" ||
			r.Header.Get("X-Media-Materials-Internal-Token") != "internal-secret" ||
			r.URL.Query().Get("organization_id") != "org-1" ||
			r.URL.Query().Get("object_path") != "materials/org-1/ai_clothing/material/movement.mp4" {
			t.Errorf("unexpected internal object request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://storage.example/fresh","expires_at":` +
			strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10) + `,"content_type":"video/mp4","size_bytes":123}`))
	}))
	defer server.Close()
	resolved, err := NewMaterialsClient(server.URL).ResolveInternalObject(context.Background(), "org-1", "materials/org-1/ai_clothing/material/movement.mp4")
	if err != nil || resolved.URL != "https://storage.example/fresh" || resolved.ContentType != "video/mp4" || resolved.SizeBytes != 123 {
		t.Fatalf("object resolution failed: result=%+v err=%v", resolved, err)
	}
}

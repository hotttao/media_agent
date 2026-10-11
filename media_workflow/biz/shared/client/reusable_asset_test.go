package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReusableAssetLookupUsesTrustedScopeAndDoesNotMaskOutage(t *testing.T) {
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "test-internal-credential")
	status := 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/internal/v1/generated-assets/reusable/gen_asset" || r.URL.Query().Get("organization_id") != "org" || r.URL.Query().Get("scope") != "media" || r.URL.Query().Get("owner_user_id") != "owner" || r.Header.Get("X-Media-Materials-Internal-Token") != "test-internal-credential" {
			t.Error("invalid internal scoped asset verification")
		}
		w.WriteHeader(status)
		if status == 200 {
			_, _ = w.Write([]byte(`{"id":"gen_asset","organization_id":"org","scope":"media","owner_user_id":"owner","job_id":"job","step_run_id":"step","template_id":"template","template_version":1,"asset_kind":"model_image","object_path":"materials/org/media/asset.png"}`))
		}
	}))
	defer server.Close()
	c := NewMaterialsClient(server.URL)
	asset, err := c.VerifyReusableGeneratedAsset(t.Context(), "gen_asset", "org", "media", "owner")
	if err != nil || asset.ID != "gen_asset" {
		t.Fatal("valid scoped asset verification failed", err)
	}
	status = 404
	if _, err := c.VerifyReusableGeneratedAsset(t.Context(), "gen_asset", "org", "media", "owner"); !errors.Is(err, ErrReusableAssetNotFound) {
		t.Fatal("deleted result did not become unavailable")
	}
	status = 503
	if _, err := c.VerifyReusableGeneratedAsset(t.Context(), "gen_asset", "org", "media", "owner"); err == nil || errors.Is(err, ErrReusableAssetNotFound) {
		t.Fatal("storage outage was converted into a cache miss")
	}
}

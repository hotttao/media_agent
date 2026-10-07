package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestArtifactPreparationUsesOnlyInternalCredential(t *testing.T) {
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "test-internal")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/internal/v1/library-objects:prepareUpload" || r.Header.Get("X-Media-Materials-Internal-Token") != "test-internal" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect internal authentication")
		}
		var in ArtifactUploadRequest
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.OrganizationID != "org" || in.Scope != "scope" || in.UploadKey != "task-key" || in.SizeBytes != 123 {
			t.Error("manifest changed")
		}
		json.NewEncoder(w).Encode(ArtifactUploadGrant{ObjectPath: "materials/org/scope/task", AlreadyUploaded: true})
	}))
	defer server.Close()
	grant, err := NewMaterialsClient(server.URL).PrepareArtifactUpload(context.Background(), ArtifactUploadRequest{OrganizationID: "org", Scope: "scope", UploadKey: "task-key", SizeBytes: 123})
	if err != nil || !grant.AlreadyUploaded || grant.ObjectPath == "" {
		t.Fatal("valid response rejected", err)
	}
}

func TestArtifactPreparationMapsErrorsWithoutLeakingResponse(t *testing.T) {
	for _, status := range []int{400, 413, 409, 401, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private-signed-url", status) }))
		_, err := NewMaterialsClient(server.URL).PrepareArtifactUpload(context.Background(), ArtifactUploadRequest{})
		server.Close()
		if err == nil || strings.Contains(err.Error(), "private-signed-url") || ((status == 400 || status == 413) && !errors.Is(err, ErrArtifactUploadRejected)) || (status == 409 && !errors.Is(err, ErrArtifactUploadConflict)) {
			t.Fatal("error mapping failed", status)
		}
	}
}

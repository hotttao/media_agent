package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveSourceMediaURL(t *testing.T) {
	origin := "https://source.example"
	for _, input := range []string{
		"/uploads/../private/image.png",
		"//other.example/uploads/image.png",
		"https://other.example/uploads/image.png",
		"/uploads/image.png?token=secret",
		"/private/image.png",
	} {
		if _, err := resolveSourceMediaURL(origin, input); err == nil {
			t.Errorf("unsafe source URL accepted: %q", input)
		}
	}
	for _, input := range []string{"/uploads/image.png", "https://source.example/uploads/image.png"} {
		got, err := resolveSourceMediaURL(origin, input)
		if err != nil || got != "https://source.example/uploads/image.png" {
			t.Errorf("source URL %q: got %q, err %v", input, got, err)
		}
	}
}

func TestImportImageThroughGatewayAndInternalAPI(t *testing.T) {
	// Minimal valid PNG header is sufficient for content-type sniffing; the
	// importer checks integrity, while the target storage verifies the object.
	image := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 64))
	digest := sha256.Sum256(image)
	checksum := hex.EncodeToString(digest[:])
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uploads/frame.png" {
			t.Errorf("unexpected source path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(image)
	}))
	defer source.Close()
	const objectPath = "materials/org/ai_clothing/generated_asset_import/frame.png"
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/library-objects:upload" || r.Header.Get("Authorization") != "Bearer public-token" || r.Header.Get("Idempotency-Key") != "media-ai-first-frame-frame-1" {
			t.Errorf("unexpected upload request: %s %s", r.Method, r.URL)
		}
		if r.URL.Query().Get("purpose") != "generated_asset_import" {
			t.Error("missing migration upload purpose")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"object_path": objectPath, "checksum": checksum})
	}))
	defer gateway.Close()
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/internal/v1/generated-assets/import" || r.Header.Get("X-Media-Materials-Migration-Token") != "migration-token" {
			t.Errorf("unexpected import request: %s %s", r.Method, r.URL)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["source_sha256"] != checksum || payload["source_record_id"] != "frame-1" || payload["object_path"] != objectPath {
			t.Errorf("unexpected import payload: %+v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "gen-1", "object_path": objectPath})
	}))
	defer internal.Close()
	var recorded []mediaArtifact
	importer := historyImporter{cfg: importConfig{
		SourceAssetsURL: source.URL, GatewayURL: gateway.URL, MaterialsURL: internal.URL,
		OrganizationID: "org", OwnerUserID: "user", ProductID: "product", VirtualIPID: "vip",
		BearerToken: "public-token", MigrationToken: "migration-token",
		OnImportedMedia: func(item mediaArtifact) { recorded = append(recorded, item) },
	}, client: http.DefaultClient}
	assetID, err := importer.importImage(context.Background(), "first_frame", sourceImage{
		ID: "frame-1", URL: "/uploads/frame.png", CreatedAt: time.Date(2026, 5, 27, 4, 51, 30, 0, time.UTC),
	})
	if err != nil || assetID != "gen-1" {
		t.Fatalf("importImage: id=%s err=%v", assetID, err)
	}
	if len(recorded) != 1 || recorded[0].TargetID != assetID || recorded[0].ObjectPath != objectPath || recorded[0].SHA256 != checksum {
		t.Fatalf("reconciliation artifact = %+v", recorded)
	}
}

func TestValidateImportConfigRejectsMissingIdentity(t *testing.T) {
	cfg := importConfig{SourceAssetsURL: "https://source.example", GatewayURL: "https://gateway.example", MaterialsURL: "http://localhost:8003"}
	if err := validateImportConfig(cfg); err == nil {
		t.Fatal("missing migration identity accepted")
	}
	cfg.OrganizationID, cfg.OwnerUserID, cfg.ProductID, cfg.VirtualIPID = "org", "user", "product", "vip"
	cfg.BearerToken, cfg.MigrationToken = "public-token", "migration-token"
	if err := validateImportConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.SourceAssetsURL = "https://source.example/base"
	if err := validateImportConfig(cfg); err == nil {
		t.Fatal("source origin with base path accepted")
	}
}

func TestImportImageDoesNotFollowSourceRedirect(t *testing.T) {
	redirected := false
	outside := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer outside.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, outside.URL+"/private", http.StatusFound)
	}))
	defer source.Close()
	importer := newHistoryImporter(importConfig{SourceAssetsURL: source.URL})
	_, err := importer.importImage(context.Background(), "first_frame", sourceImage{
		ID: "frame", URL: "/uploads/frame.png", CreatedAt: time.Now(),
	})
	if err == nil || redirected {
		t.Fatalf("source redirect accepted or followed: err=%v redirected=%v", err, redirected)
	}
}

func TestIncompleteImportReplayAcceptsAdvancedJobState(t *testing.T) {
	for _, status := range []int{2, 4, 7, 8} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"job_id": "existing-job", "status": status})
			}))
			defer server.Close()
			h := newHistoryImporter(importConfig{WorkflowURL: server.URL})
			id, err := h.importHistoricalJob(context.Background(), "/import", "frame:sample", nil, 7)
			if status <= 7 && (err != nil || id != "existing-job") {
				t.Fatalf("existing Job state %d rejected: %v", status, err)
			}
			if status > 7 && err == nil {
				t.Fatal("invalid Job state accepted")
			}
		})
	}
}

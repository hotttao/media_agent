package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func validCompletedVideoManifest() manifest {
	created := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	m := manifest{
		ModelImages: []sourceImage{{ID: "model", ProductID: "product", VirtualIPID: "ip", URL: "/uploads/model.png", CreatedAt: created}},
		StyleImages: []sourceImage{{ID: "style", ProductID: "product", VirtualIPID: "ip", ParentID: "model", URL: "/uploads/style.png", CreatedAt: created}},
		FirstFrames: []sourceImage{{ID: "frame", ProductID: "product", VirtualIPID: "ip", ParentID: "style", URL: "/uploads/frame.png", CreatedAt: created}},
		Tasks:       []videoTask{{ID: "task", Videos: []video{{ID: "video", TaskID: "task", ProductID: "product", IPID: "ip", ModelImageID: "model", StyleImageID: "style", FirstFrameID: "frame", URL: "/uploads/video.mp4", CreatedAt: created}}}},
	}
	buildLineages(&m, "")
	buildGroups(&m, "")
	return m
}

func TestVideoAssetAndCompletedJobUseStableSourceIdentity(t *testing.T) {
	payload := []byte("\x00\x00\x00\x18ftypisom" + strings.Repeat("x", 64))
	digest := sha256.Sum256(payload)
	checksum := hex.EncodeToString(digest[:])
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uploads/video.mp4" {
			t.Errorf("unexpected source media path %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer source.Close()
	const objectPath = "materials/org/ai_clothing/generated_asset_import/video.mp4"
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "media-ai-ai-video-video" || r.Header.Get("Authorization") != "Bearer public-token" {
			t.Errorf("unstable video upload identity: %s", r.Header.Get("Idempotency-Key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"object_path": objectPath, "checksum": checksum})
	}))
	defer gateway.Close()
	materials := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Media-Materials-Migration-Token") != "materials-migration" {
			t.Error("video asset was not imported with migration identity")
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request["asset_kind"] != "ai_video" || request["source_record_id"] != "video" || request["source_sha256"] != checksum {
			t.Errorf("incorrect video asset facts: %+v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "gen-video", "object_path": objectPath})
	}))
	defer materials.Close()
	workflow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/internal/v1/jobs/import-completed" || r.Header.Get("X-Media-Workflow-Migration-Token") != "workflow-migration" {
			t.Errorf("wrong terminal Job endpoint or identity: %s", r.URL.Path)
		}
		var request struct {
			SourceGroupID string              `json:"source_group_id"`
			Completed     []map[string]string `json:"completed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.SourceGroupID != "video:video" || len(request.Completed) != 4 || request.Completed[3]["asset_id"] != "gen-video" {
			t.Errorf("wrong completed Job lineage: %+v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"job_id": "job-video", "status": 4})
	}))
	defer workflow.Close()
	importer := newHistoryImporter(importConfig{SourceAssetsURL: source.URL, GatewayURL: gateway.URL, MaterialsURL: materials.URL,
		WorkflowURL: workflow.URL, OrganizationID: "org", OwnerUserID: "owner", ProductID: "product", VirtualIPID: "vip",
		WorkspaceID: "workspace", WorkflowID: "wf", WorkflowVersion: 1, BearerToken: "public-token",
		MigrationToken: "materials-migration", WorkflowToken: "workflow-migration"})
	assetID, err := importer.importMedia(context.Background(), "ai_video", "video", "/uploads/video.mp4", time.Now(), "video/")
	if err != nil || assetID != "gen-video" {
		t.Fatalf("video asset import failed: id=%s err=%v", assetID, err)
	}
	completed := []map[string]string{{"step_key": "model_image", "asset_id": "m"}, {"step_key": "style_image", "asset_id": "s"}, {"step_key": "first_frame", "asset_id": "f"}, {"step_key": "video", "asset_id": assetID}}
	jobID, err := importer.importHistoricalJob(context.Background(), "/api/internal/v1/jobs/import-completed", "video:video", completed, 4)
	if err != nil || jobID != "job-video" {
		t.Fatalf("completed Job import failed: id=%s err=%v", jobID, err)
	}
}

func TestCompletedVideoSourceRequiresIntactCandidate(t *testing.T) {
	m := validCompletedVideoManifest()
	lineage, source, model, style, frame, err := completedVideoSource(&m, "video")
	if err != nil || lineage.VideoID != "video" || source.TaskID != "task" || model.ID != "model" || style.ID != "style" || frame.ID != "frame" {
		t.Fatalf("intact Video lineage rejected: lineage=%+v video=%+v err=%v", lineage, source, err)
	}
	m.Excluded = append(m.Excluded, lineageExclusion{Kind: "video", SourceID: "video", Reason: "bad_media"})
	if _, _, _, _, _, err := completedVideoSource(&m, "video"); err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("excluded Video accepted: %v", err)
	}
	m = validCompletedVideoManifest()
	m.FirstFrames[0].ParentID = "other-style"
	if _, _, _, _, _, err := completedVideoSource(&m, "video"); err == nil {
		t.Fatal("disconnected FirstFrame accepted")
	}
	m = validCompletedVideoManifest()
	m.Groups = nil
	if _, _, _, _, _, err := completedVideoSource(&m, "video"); err == nil {
		t.Fatal("Video without intact candidate group accepted")
	}
}

func TestDetectSourceMediaTypeHandlesMP4AndWebM(t *testing.T) {
	if got := detectSourceMediaType([]byte("\x00\x00\x00\x18ftypisom0000")); got != "video/mp4" {
		t.Fatalf("MP4 type = %q", got)
	}
	if got := detectSourceMediaType([]byte("\x1a\x45\xdf\xa3data")); got != "video/webm" {
		t.Fatalf("WebM type = %q", got)
	}
}

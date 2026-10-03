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
)

func TestIntactSourceVideoPushKeepsOriginalMemberOrder(t *testing.T) {
	m := manifest{
		VideoPushes: []sourceVideoPush{{ID: "push", VideoIDs: []string{"video-b", "video-a"}}},
		Groups:      []lineageGroup{{ID: "push:push", Kind: "video_push", VideoPushID: "push", VideoIDs: []string{"video-a", "video-b"}}},
	}
	push, err := intactSourceVideoPush(&m, "push")
	if err != nil || strings.Join(push.VideoIDs, ",") != "video-b,video-a" {
		t.Fatalf("VideoPush order was replaced by sorted group order: push=%+v err=%v", push, err)
	}
	m.Excluded = []lineageExclusion{{Kind: "video_push", SourceID: "push", Reason: "missing_video_reference"}}
	if _, err := intactSourceVideoPush(&m, "push"); err == nil {
		t.Fatal("excluded VideoPush accepted")
	}
}

func TestImportPublicationMediaUsesDedicatedPurposeAndChecksum(t *testing.T) {
	payload := []byte("\x00\x00\x00\x18ftypisom" + strings.Repeat("x", 48))
	digest := sha256.Sum256(payload)
	checksum := hex.EncodeToString(digest[:])
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uploads/clip.mp4" {
			t.Errorf("unexpected source path %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer source.Close()
	const targetPath = "materials/org/ai_clothing/historical_publication_import/clip.mp4"
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("purpose") != "historical_publication_import" || r.Header.Get("Idempotency-Key") != "media-ai-push-video-push" {
			t.Errorf("wrong import purpose or idempotency key: %s", r.URL)
		}
		if r.ContentLength <= int64(len(payload)) {
			t.Errorf("multipart body was not sized: %d", r.ContentLength)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"object_path": targetPath, "checksum": checksum})
	}))
	defer gateway.Close()
	var recorded []mediaArtifact
	importer := newHistoryImporter(importConfig{SourceAssetsURL: source.URL, GatewayURL: gateway.URL, OrganizationID: "org", BearerToken: "token", OnImportedMedia: func(item mediaArtifact) { recorded = append(recorded, item) }})
	media, err := importer.importPublicationMedia(context.Background(), "push", "video", "/uploads/clip.mp4", "video/")
	if err != nil || media.ObjectPath != targetPath || media.SHA256 != checksum {
		t.Fatalf("publication media import failed: media=%+v err=%v", media, err)
	}
	missing, err := importer.importPublicationMedia(context.Background(), "push", "thumbnail", "", "image/")
	if err != nil || missing.ObjectPath != "" || missing.SHA256 != "" {
		t.Fatalf("empty media field got a fake object: media=%+v err=%v", missing, err)
	}
	if len(recorded) != 1 || recorded[0].ObjectPath != targetPath || recorded[0].SHA256 != checksum || recorded[0].Kind != "video_push_video" {
		t.Fatalf("reconciliation artifact = %+v", recorded)
	}
}

func TestImportPublicationThumbnailAcceptsImageAndRejectsVideo(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 48))
	digest := sha256.Sum256(png)
	checksum := hex.EncodeToString(digest[:])
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(png)
	}))
	defer source.Close()
	const targetPath = "materials/org/ai_clothing/historical_publication_import/thumb.png"
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("purpose") != "historical_publication_import" || r.Header.Get("Idempotency-Key") != "media-ai-push-thumbnail-push" {
			t.Errorf("wrong thumbnail upload identity: %s", r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"object_path": targetPath, "checksum": checksum})
	}))
	defer gateway.Close()
	importer := newHistoryImporter(importConfig{SourceAssetsURL: source.URL, GatewayURL: gateway.URL, OrganizationID: "org", BearerToken: "token"})
	thumbnail, err := importer.importPublicationMedia(context.Background(), "push", "thumbnail", "/uploads/thumb.png", "image/")
	if err != nil || thumbnail.ObjectPath != targetPath || thumbnail.SHA256 != checksum {
		t.Fatalf("thumbnail import failed: %+v %v", thumbnail, err)
	}
	if _, err := importer.importPublicationMedia(context.Background(), "push", "manual-clip", "/uploads/thumb.png", "video/"); err == nil {
		t.Fatal("manual clip accepted an image")
	}
}

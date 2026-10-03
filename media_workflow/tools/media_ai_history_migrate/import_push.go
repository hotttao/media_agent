package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

type importedPushMedia struct {
	ObjectPath string `json:"object_path"`
	SHA256     string `json:"sha256"`
}

func (h historyImporter) importPublicationMedia(ctx context.Context, pushID, field, rawURL, mediaPrefix string) (importedPushMedia, error) {
	if rawURL == "" {
		return importedPushMedia{}, nil
	}
	sourceURL, err := resolveSourceMediaURL(h.cfg.SourceAssetsURL, rawURL)
	if err != nil {
		return importedPushMedia{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return importedPushMedia{}, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return importedPushMedia{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return importedPushMedia{}, fmt.Errorf("source VideoPush %s media HTTP %d", field, resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "media-ai-push-*")
	if err != nil {
		return importedPushMedia{}, err
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(resp.Body, maxImportMediaBytes+1))
	if err != nil || size == 0 || size > maxImportMediaBytes {
		return importedPushMedia{}, errors.New("source VideoPush media unreadable, empty, or exceeds 256 MiB")
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return importedPushMedia{}, err
	}
	header := make([]byte, 512)
	n, err := io.ReadFull(tmp, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return importedPushMedia{}, err
	}
	contentType := detectSourceMediaType(header[:n])
	if !strings.HasPrefix(contentType, mediaPrefix) || mediaPrefix == "image/" && size > maxImportImageBytes {
		return importedPushMedia{}, fmt.Errorf("source VideoPush %s media type or size invalid: %s", field, contentType)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return importedPushMedia{}, err
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	objectPath, err := h.uploadWithPurpose(ctx, "historical_publication_import", "push-"+field, pushID, path.Base(rawURL), tmp, checksum)
	if err != nil {
		return importedPushMedia{}, err
	}
	return importedPushMedia{ObjectPath: objectPath, SHA256: checksum}, nil
}

func intactSourceVideoPush(m *manifest, pushID string) (sourceVideoPush, error) {
	for _, excluded := range m.Excluded {
		if excluded.Kind == "video_push" && excluded.SourceID == pushID {
			return sourceVideoPush{}, fmt.Errorf("VideoPush %s is excluded: %s", pushID, excluded.Reason)
		}
	}
	for _, group := range m.Groups {
		if group.Kind != "video_push" || group.VideoPushID != pushID {
			continue
		}
		for _, push := range m.VideoPushes {
			if push.ID == pushID && len(push.VideoIDs) > 0 {
				return push, nil
			}
		}
	}
	return sourceVideoPush{}, fmt.Errorf("VideoPush %s has no intact candidate group", pushID)
}

func importHistoricalVideoPush(ctx context.Context, db *sql.DB, m *manifest, pushID string, cfg importConfig) (string, error) {
	return importHistoricalVideoPushWithJobs(ctx, db, m, pushID, cfg, nil)
}

// alreadyImported is populated by the full-batch path. A single-Push import
// still imports each member itself, while batch import reuses confirmed Jobs.
func importHistoricalVideoPushWithJobs(ctx context.Context, db *sql.DB, m *manifest, pushID string, cfg importConfig, alreadyImported map[string]string) (string, error) {
	push, err := intactSourceVideoPush(m, pushID)
	if err != nil {
		return "", err
	}
	if err := validateImportConfig(cfg); err != nil {
		return "", err
	}
	if cfg.WorkflowURL == "" || cfg.WorkflowToken == "" || cfg.WorkflowID == "" || cfg.WorkflowVersion <= 0 || cfg.WorkspaceID == "" {
		return "", errors.New("VideoPush import requires workflow migration identity, published workflow, and Workspace")
	}
	if err := validateOrigin("workflow-internal-url", cfg.WorkflowURL); err != nil {
		return "", err
	}
	importer := newHistoryImporter(cfg)
	members := make([]map[string]any, 0, len(push.VideoIDs))
	seen := map[string]bool{}
	for _, videoID := range push.VideoIDs {
		if seen[videoID] {
			return "", fmt.Errorf("VideoPush %s repeats Video %s", pushID, videoID)
		}
		seen[videoID] = true
		jobID := alreadyImported[videoID]
		if jobID == "" {
			var err error
			jobID, err = importCompletedVideoID(ctx, db, m, videoID, cfg)
			if err != nil {
				return "", fmt.Errorf("VideoPush member %s: %w", videoID, err)
			}
		}
		members = append(members, map[string]any{"ordinal": len(members), "source_video_id": videoID, "job_id": jobID})
	}
	videoMedia, err := importer.importPublicationMedia(ctx, pushID, "video", push.URL, "video/")
	if err != nil {
		return "", fmt.Errorf("VideoPush video: %w", err)
	}
	thumbnail, err := importer.importPublicationMedia(ctx, pushID, "thumbnail", push.Thumbnail, "image/")
	if err != nil {
		return "", fmt.Errorf("VideoPush thumbnail: %w", err)
	}
	manualClip, err := importer.importPublicationMedia(ctx, pushID, "manual-clip", push.ManualClipURL, "video/")
	if err != nil {
		return "", fmt.Errorf("VideoPush manual clip: %w", err)
	}
	if push.CreatedAt.IsZero() || push.UpdatedAt.IsZero() {
		return "", errors.New("VideoPush source time is missing")
	}
	request := map[string]any{
		"organization_id": cfg.OrganizationID, "workspace_id": cfg.WorkspaceID, "owner_user_id": cfg.OwnerUserID,
		"source_record_id": push.ID, "product_id": cfg.ProductID, "virtual_ip_id": cfg.VirtualIPID,
		"source_scene_id": push.SceneID, "template_name": push.TemplateName, "title": push.Title, "content": push.Content,
		"source_status": push.Status, "is_qualified": push.IsQualified, "is_published": push.IsPublished,
		"video": videoMedia, "thumbnail": thumbnail, "manual_clip": manualClip, "members": members,
		"source_created_at": push.CreatedAt.UTC().Format(time.RFC3339Nano), "source_updated_at": push.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WorkflowURL+"/api/internal/v1/historical-video-pushes/import", bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-Workflow-Migration-Token", cfg.WorkflowToken)
	var result struct {
		Item struct {
			ID             string `json:"id"`
			SourceRecordID string `json:"source_record_id"`
		} `json:"item"`
	}
	if err := importer.do(req, &result); err != nil {
		return "", err
	}
	if result.Item.ID == "" || result.Item.SourceRecordID != push.ID {
		return "", errors.New("target did not confirm Historical VideoPush")
	}
	fmt.Printf("imported Historical VideoPush: source_id=%s target_id=%s members=%d\n", push.ID, result.Item.ID, len(members))
	return result.Item.ID, nil
}

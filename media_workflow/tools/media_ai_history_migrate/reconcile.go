package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type reconciledJob struct {
	existingHistoryJob
	Status     int   `json:"status"`
	FinishedAt int64 `json:"finished_at"`
}
type reconciledStep struct {
	Key             string `json:"step_key"`
	Status          int    `json:"status"`
	Origin          string `json:"origin"`
	SourceID        string `json:"source_record_id"`
	AssetID         string `json:"output_asset_id"`
	SourceCreatedAt int64  `json:"source_created_at"`
}

func sourceHistoryArtifacts(m *manifest, kind, sourceID string) (map[string]sourceImage, error) {
	if kind == "video" {
		_, video, model, style, frame, err := completedVideoSource(m, sourceID)
		if err != nil {
			return nil, err
		}
		return map[string]sourceImage{"model_image": model, "style_image": style, "first_frame": frame, "ai_video": {ID: video.ID, CreatedAt: video.CreatedAt}}, nil
	}
	for _, group := range m.Groups {
		if group.ID == sourceID && group.ExecutionState == "awaiting_resume" {
			want := map[string]sourceImage{}
			for _, item := range []struct {
				kind, id string
				images   []sourceImage
			}{{"model_image", group.ModelImageID, m.ModelImages}, {"style_image", group.StyleImageID, m.StyleImages}, {"first_frame", group.FirstFrameID, m.FirstFrames}} {
				if item.id == "" {
					continue
				}
				image, found := findSourceImage(item.images, item.id)
				if !found {
					return nil, fmt.Errorf("missing source image %s", item.id)
				}
				want[item.kind] = image
			}
			return want, nil
		}
	}
	return nil, fmt.Errorf("unknown source history %s", sourceID)
}

// reconcileHistory reads source inventory, report and target APIs. All expected
// source records must occur exactly once; failed records remain quarantined.
func reconcileHistory(ctx context.Context, m *manifest, cfg importConfig, reportPath string) error {
	file, err := os.Open(reportPath)
	if err != nil {
		return err
	}
	defer file.Close()
	if cfg.BearerToken == "" || cfg.OrganizationID == "" || cfg.WorkspaceID == "" || cfg.OwnerUserID == "" {
		return errors.New("reconciliation requires API Token, organization, owner and Workspace")
	}
	expected := map[string]bool{}
	for _, group := range m.Groups {
		if group.ExecutionState == "awaiting_resume" {
			expected["incomplete_group:"+group.ID] = true
		}
		if group.Kind == "video_push" {
			expected["video_push:"+group.VideoPushID] = true
		}
		for _, id := range group.VideoIDs {
			expected["video:"+id] = true
		}
	}
	for _, item := range m.Excluded {
		expected["excluded:"+item.Kind+":"+item.SourceID] = true
	}
	seen := map[string]bool{}
	objects := map[string]string{}
	jobsByVideo := map[string]string{}
	h := newHistoryImporter(cfg)
	imported, quarantined, excluded := 0, 0, 0
	var pendingPushes []batchResult
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		var row batchResult
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return fmt.Errorf("invalid reconciliation row: %w", err)
		}
		key := row.Kind + ":" + row.SourceID
		if row.Status == "excluded" {
			key = "excluded:" + key
		}
		if !expected[key] || seen[key] {
			return fmt.Errorf("unexpected or duplicate report record %s", key)
		}
		seen[key] = true
		switch row.Status {
		case "excluded":
			excluded++
			continue
		case "failed":
			quarantined++
			fmt.Printf("quarantined source: kind=%s source_id=%s\n", row.Kind, row.SourceID)
			continue
		case "imported":
		default:
			return fmt.Errorf("unknown report status %s", row.Status)
		}
		if row.TargetID == "" {
			return fmt.Errorf("imported %s has no target ID", key)
		}
		for _, object := range row.Artifacts {
			if object.ObjectPath == "" || len(object.SHA256) != 64 {
				return fmt.Errorf("invalid object facts in %s", key)
			}
			if digest, found := objects[object.ObjectPath]; found && digest != object.SHA256 {
				return fmt.Errorf("conflicting object digest in %s", key)
			}
			objects[object.ObjectPath] = object.SHA256
		}
		if row.Kind == "video_push" {
			pendingPushes = append(pendingPushes, row)
			imported++
			continue
		}
		var response struct {
			Job   reconciledJob    `json:"job"`
			Steps []reconciledStep `json:"step_runs"`
		}
		if err := h.getTarget(ctx, "/api/v1/jobs/"+url.PathEscape(row.TargetID), &response); err != nil {
			return err
		}
		job := response.Job
		sourceKey := row.SourceID
		if row.Kind == "video" {
			sourceKey = "video:" + sourceKey
			jobsByVideo[row.SourceID] = row.TargetID
		}
		if job.ID != row.TargetID || job.SourceRecordID != sourceKey || job.SourceSystem != "media_ai" || job.Origin != "legacy_import" || job.OrganizationID != cfg.OrganizationID || job.WorkspaceID != cfg.WorkspaceID || job.SubmittedBy != cfg.OwnerUserID || len(response.Steps) != 4 {
			return fmt.Errorf("target Job identity mismatch for %s", key)
		}
		if row.Kind == "video" && job.Status != 4 {
			return fmt.Errorf("historical Video Job is not succeeded: %s", key)
		}
		wantSteps := map[string]mediaArtifact{}
		wantSources, err := sourceHistoryArtifacts(m, row.Kind, row.SourceID)
		if err != nil {
			return err
		}
		if len(row.Artifacts) != len(wantSources) {
			return fmt.Errorf("report artifact count differs from source: %s", key)
		}
		if row.Kind == "video" && job.FinishedAt != wantSources["ai_video"].CreatedAt.UnixNano() {
			return fmt.Errorf("historical video completion time differs from source: %s", key)
		}
		for _, object := range row.Artifacts {
			source, found := wantSources[object.Kind]
			if !found || source.ID != object.SourceID {
				return fmt.Errorf("report artifact differs from source lineage: %s", key)
			}
			stepKey := object.Kind
			if stepKey == "ai_video" {
				stepKey = "video"
			}
			wantSteps[stepKey] = object
			var asset struct {
				ID           string `json:"id"`
				Origin       string `json:"origin"`
				SourceSystem string `json:"source_system"`
				SourceID     string `json:"source_record_id"`
				ObjectPath   string `json:"object_path"`
				Kind         string `json:"asset_kind"`
			}
			if err := h.getTarget(ctx, "/api/v1/generated-assets/"+url.PathEscape(object.TargetID)+"?organization_id="+url.QueryEscape(cfg.OrganizationID), &asset); err != nil {
				return err
			}
			if asset.ID != object.TargetID || asset.Origin != "legacy_import" || asset.SourceSystem != "media_ai" || asset.SourceID != object.SourceID || asset.ObjectPath != object.ObjectPath || asset.Kind != object.Kind {
				return fmt.Errorf("target Generated Asset mismatch for %s", key)
			}
		}
		matched := 0
		for _, step := range response.Steps {
			if want, found := wantSteps[step.Key]; found {
				matched++
				if step.Status != 5 || step.Origin != "legacy_import" || step.SourceID != want.SourceID || step.AssetID != want.TargetID || step.SourceCreatedAt != wantSources[want.Kind].CreatedAt.UnixNano() {
					return fmt.Errorf("historical StepRun mismatch for %s/%s", key, step.Key)
				}
			}
		}
		if matched != len(wantSteps) || len(wantSteps) == 0 {
			return fmt.Errorf("historical prefix incomplete for %s", key)
		}
		imported++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for key := range expected {
		if !seen[key] {
			return fmt.Errorf("source record absent from report: %s", key)
		}
	}
	pushes := map[string]sourceVideoPush{}
	for _, push := range m.VideoPushes {
		pushes[push.ID] = push
	}
	for _, row := range pendingPushes {
		source := pushes[row.SourceID]
		var response struct {
			Item struct {
				ID           string    `json:"id"`
				SourceID     string    `json:"source_record_id"`
				Workspace    string    `json:"workspace_id"`
				Organization string    `json:"organization_id"`
				Owner        string    `json:"owner_user_id"`
				Scene        string    `json:"source_scene_id"`
				Template     string    `json:"template_name"`
				Title        string    `json:"title"`
				Content      string    `json:"content"`
				Status       string    `json:"source_status"`
				Qualified    bool      `json:"is_qualified"`
				Published    bool      `json:"is_published"`
				Created      time.Time `json:"source_created_at"`
				Updated      time.Time `json:"source_updated_at"`
				Members      []struct {
					Ordinal  int    `json:"ordinal"`
					SourceID string `json:"source_video_id"`
					JobID    string `json:"job_id"`
				} `json:"members"`
				Video     importedPushMedia `json:"video"`
				Thumbnail importedPushMedia `json:"thumbnail"`
				Manual    importedPushMedia `json:"manual_clip"`
			} `json:"item"`
		}
		if err := h.getTarget(ctx, "/api/v1/historical-video-pushes/"+url.PathEscape(row.TargetID), &response); err != nil {
			return err
		}
		item := response.Item
		if item.ID != row.TargetID || item.SourceID != source.ID || item.Workspace != cfg.WorkspaceID || item.Organization != cfg.OrganizationID || item.Owner != cfg.OwnerUserID || item.Scene != source.SceneID || item.Template != source.TemplateName || item.Title != source.Title || item.Content != source.Content || item.Status != source.Status || item.Qualified != source.IsQualified || item.Published != source.IsPublished || !item.Created.Equal(source.CreatedAt) || !item.Updated.Equal(source.UpdatedAt) || len(item.Members) != len(source.VideoIDs) {
			return fmt.Errorf("historical publication facts mismatch: %s", row.SourceID)
		}
		for i, member := range item.Members {
			if member.Ordinal != i || member.SourceID != source.VideoIDs[i] || member.JobID != jobsByVideo[member.SourceID] {
				return fmt.Errorf("historical publication members mismatch: %s", row.SourceID)
			}
		}
		for _, field := range []struct {
			kind, raw string
			object    importedPushMedia
		}{{"video_push_video", source.URL, item.Video}, {"video_push_thumbnail", source.Thumbnail, item.Thumbnail}, {"video_push_manual-clip", source.ManualClipURL, item.Manual}} {
			if field.raw == "" {
				if field.object.ObjectPath != "" || field.object.SHA256 != "" {
					return fmt.Errorf("source empty media became nonempty: %s", row.SourceID)
				}
				continue
			}
			found := false
			for _, artifact := range row.Artifacts {
				if artifact.Kind == field.kind && artifact.ObjectPath == field.object.ObjectPath && artifact.SHA256 == field.object.SHA256 {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("historical publication media mismatch: %s/%s", row.SourceID, field.kind)
			}
		}
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for path, digest := range objects {
		var resolved struct {
			URL string `json:"url"`
		}
		if err := h.getTarget(ctx, "/api/v1/library-objects:resolve?organization_id="+url.QueryEscape(cfg.OrganizationID)+"&object_path="+url.QueryEscape(path), &resolved); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved.URL, nil)
		if err != nil {
			return errors.New("invalid resolved media URL")
		}
		req.Header.Set("Range", "bytes=0-0")
		resp, err := client.Do(req)
		if err != nil {
			return errors.New("target object probe failed")
		}
		_, readErr := io.CopyN(io.Discard, resp.Body, 1)
		resp.Body.Close()
		if (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent) || readErr != nil || strings.ToLower(resp.Header.Get("X-Amz-Meta-Checksum-Sha256")) != digest {
			return fmt.Errorf("target object unreadable or checksum mismatch: %s", path)
		}
	}
	fmt.Printf("reconciliation verified: imported=%d excluded=%d quarantined=%d unique_objects=%d\n", imported, excluded, quarantined, len(objects))
	if quarantined > 0 {
		return fmt.Errorf("%d source records remain quarantined", quarantined)
	}
	return nil
}

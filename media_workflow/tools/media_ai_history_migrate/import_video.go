package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// completedVideoSource accepts one Video only when the full source inventory
// placed it in an intact candidate group. A VideoPush remains a separate
// multi-video fact and is never used as a single-output Job identity.
func completedVideoSource(m *manifest, videoID string) (videoLineage, video, sourceImage, sourceImage, sourceImage, error) {
	if videoID == "" {
		return videoLineage{}, video{}, sourceImage{}, sourceImage{}, sourceImage{}, errors.New("source Video ID is required")
	}
	for _, excluded := range m.Excluded {
		if excluded.Kind == "video" && excluded.SourceID == videoID {
			return videoLineage{}, video{}, sourceImage{}, sourceImage{}, sourceImage{}, fmt.Errorf("source Video %s is excluded: %s", videoID, excluded.Reason)
		}
	}
	candidate := false
	for _, group := range m.Groups {
		for _, id := range group.VideoIDs {
			if id == videoID {
				candidate = true
				break
			}
		}
	}
	if !candidate {
		return videoLineage{}, video{}, sourceImage{}, sourceImage{}, sourceImage{}, fmt.Errorf("source Video %s has no intact candidate group", videoID)
	}
	var lineage videoLineage
	foundLineage := false
	for _, item := range m.Lineages {
		if item.VideoID == videoID {
			lineage, foundLineage = item, true
			break
		}
	}
	var source video
	foundVideo := false
	for _, task := range m.Tasks {
		for _, item := range task.Videos {
			if item.ID == videoID {
				source, foundVideo = item, true
				if item.TaskID != task.ID || lineage.TaskID != task.ID {
					return videoLineage{}, video{}, sourceImage{}, sourceImage{}, sourceImage{}, errors.New("VideoTask relation differs from source lineage")
				}
				break
			}
		}
	}
	if !foundLineage || !foundVideo || source.URL == "" || source.CreatedAt.IsZero() ||
		source.ProductID == "" || source.IPID == "" ||
		lineage.ProductID != source.ProductID || lineage.VirtualIPID != source.IPID ||
		lineage.ModelImageID != source.ModelImageID || lineage.StyleImageID != source.StyleImageID || lineage.FirstFrameID != source.FirstFrameID {
		return videoLineage{}, video{}, sourceImage{}, sourceImage{}, sourceImage{}, errors.New("Video has incomplete or inconsistent source lineage")
	}
	model, hasModel := findSourceImage(m.ModelImages, source.ModelImageID)
	style, hasStyle := findSourceImage(m.StyleImages, source.StyleImageID)
	frame, hasFrame := findSourceImage(m.FirstFrames, source.FirstFrameID)
	if !hasModel || !hasStyle || !hasFrame ||
		model.ProductID != source.ProductID || model.VirtualIPID != source.IPID ||
		style.ParentID != model.ID || frame.ParentID != style.ID ||
		style.ProductID != "" && style.ProductID != source.ProductID || style.VirtualIPID != "" && style.VirtualIPID != source.IPID ||
		frame.ProductID != "" && frame.ProductID != source.ProductID || frame.VirtualIPID != "" && frame.VirtualIPID != source.IPID {
		return videoLineage{}, video{}, sourceImage{}, sourceImage{}, sourceImage{}, errors.New("Video upstream images are disconnected")
	}
	return lineage, source, model, style, frame, nil
}

func importCompletedVideo(ctx context.Context, db *sql.DB, m *manifest, videoID string, cfg importConfig) error {
	if err := validateImportConfig(cfg); err != nil {
		return err
	}
	if cfg.WorkflowURL == "" || cfg.WorkflowToken == "" || cfg.WorkflowID == "" || cfg.WorkflowVersion <= 0 || cfg.WorkspaceID == "" {
		return errors.New("--import-video requires target workflow URL, migration token, published workflow ID/version, and target Workspace ID")
	}
	if err := validateOrigin("workflow-internal-url", cfg.WorkflowURL); err != nil {
		return err
	}
	lineage, source, model, style, frame, err := completedVideoSource(m, videoID)
	if err != nil {
		return err
	}
	importer := newHistoryImporter(cfg)
	if err := importer.verifyInputMappings(ctx, db, lineageGroup{ProductID: lineage.ProductID, VirtualIPID: lineage.VirtualIPID}); err != nil {
		return err
	}
	images := []struct {
		kind  string
		image sourceImage
	}{{"model_image", model}, {"style_image", style}, {"first_frame", frame}}
	completed := make([]map[string]string, 0, 4)
	for _, item := range images {
		assetID, err := importer.importImage(ctx, item.kind, item.image)
		if err != nil {
			return fmt.Errorf("%s %s: %w", item.kind, item.image.ID, err)
		}
		completed = append(completed, map[string]string{"step_key": item.kind, "source_record_id": item.image.ID, "asset_id": assetID})
		fmt.Printf("imported historical asset: kind=%s source_id=%s target_id=%s\n", item.kind, item.image.ID, assetID)
	}
	assetID, err := importer.importMedia(ctx, "ai_video", source.ID, source.URL, source.CreatedAt, "video/")
	if err != nil {
		return fmt.Errorf("video %s: %w", source.ID, err)
	}
	completed = append(completed, map[string]string{"step_key": "video", "source_record_id": source.ID, "asset_id": assetID})
	fmt.Printf("imported historical asset: kind=ai_video source_id=%s target_id=%s\n", source.ID, assetID)
	jobID, err := importer.importHistoricalJob(ctx, "/api/internal/v1/jobs/import-completed", "video:"+source.ID, completed, 4)
	if err != nil {
		return fmt.Errorf("historical assets imported, but completed Job import failed: %w", err)
	}
	fmt.Printf("imported completed Job: video_id=%s job_id=%s status=succeeded\n", source.ID, jobID)
	return nil
}

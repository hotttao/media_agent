package main

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

type sourceImage struct {
	ID             string    `json:"id"`
	ProductID      string    `json:"product_id"`
	VirtualIPID    string    `json:"virtual_ip_id"`
	ParentID       string    `json:"parent_id,omitempty"`
	URL            string    `json:"url"`
	Prompt         string    `json:"prompt,omitempty"`
	InputHash      string    `json:"input_hash"`
	Outfit         string    `json:"outfit,omitempty"`
	PoseID         string    `json:"pose_id,omitempty"`
	MakeupID       string    `json:"makeup_id,omitempty"`
	AccessoryID    string    `json:"accessory_id,omitempty"`
	SceneID        string    `json:"scene_id,omitempty"`
	Composition    string    `json:"composition,omitempty"`
	GenerationPath string    `json:"generation_path,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type sourceVideoPush struct {
	ID            string    `json:"id"`
	VideoIDs      []string  `json:"video_ids"`
	ProductID     string    `json:"product_id"`
	VirtualIPID   string    `json:"virtual_ip_id"`
	SceneID       string    `json:"scene_id"`
	TemplateName  string    `json:"template_name,omitempty"`
	URL           string    `json:"url,omitempty"`
	Thumbnail     string    `json:"thumbnail,omitempty"`
	Title         string    `json:"title,omitempty"`
	Content       string    `json:"content,omitempty"`
	ManualClipURL string    `json:"manual_clip_url,omitempty"`
	Status        string    `json:"status"`
	IsQualified   bool      `json:"is_qualified"`
	IsPublished   bool      `json:"is_published"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type videoLineage struct {
	VideoID      string   `json:"video_id"`
	TaskID       string   `json:"task_id"`
	ProductID    string   `json:"product_id"`
	VirtualIPID  string   `json:"virtual_ip_id"`
	ModelImageID string   `json:"model_image_id"`
	StyleImageID string   `json:"style_image_id"`
	FirstFrameID string   `json:"first_frame_id"`
	PoseID       string   `json:"pose_id,omitempty"`
	SceneID      string   `json:"scene_id,omitempty"`
	MovementID   string   `json:"movement_id"`
	TemplateIDs  []string `json:"matching_workflow_template_ids"`
	VideoPushIDs []string `json:"video_push_ids"`
}

type lineageIssue struct {
	Kind     string `json:"kind"`
	SourceID string `json:"source_id"`
	RefID    string `json:"ref_id,omitempty"`
}

type lineageExclusion struct {
	Kind     string `json:"kind"`
	SourceID string `json:"source_id"`
	Reason   string `json:"reason"`
}

// A group is a projection of source relationships, not a target Job.
// Shared image records stay outside groups and are referenced by ID.
type lineageGroup struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	ExecutionState string   `json:"execution_state,omitempty"`
	NextStepKey    string   `json:"next_step_key,omitempty"`
	VideoPushID    string   `json:"video_push_id,omitempty"`
	ProductID      string   `json:"product_id,omitempty"`
	VirtualIPID    string   `json:"virtual_ip_id,omitempty"`
	ModelImageID   string   `json:"model_image_id,omitempty"`
	StyleImageID   string   `json:"style_image_id,omitempty"`
	FirstFrameID   string   `json:"first_frame_id,omitempty"`
	VideoIDs       []string `json:"video_ids,omitempty"`
	TemplateIDs    []string `json:"matching_workflow_template_ids,omitempty"`
}

func readLineage(ctx context.Context, db *sql.DB, m *manifest, onlyTask string) error {
	rows, err := db.QueryContext(ctx, `SELECT id, product_id, ip_id, url, COALESCE(prompt,''), input_hash, outfit, created_at FROM model_images ORDER BY id`)
	if err != nil {
		return fmt.Errorf("model_images: %w", err)
	}
	for rows.Next() {
		var x sourceImage
		if err := rows.Scan(&x.ID, &x.ProductID, &x.VirtualIPID, &x.URL, &x.Prompt, &x.InputHash, &x.Outfit, &x.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		m.ModelImages = append(m.ModelImages, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT id, product_id, ip_id, model_image_id, url, COALESCE(prompt,''), input_hash, pose_id, makeup_id, accessory_id, created_at FROM style_images ORDER BY id`)
	if err != nil {
		return fmt.Errorf("style_images: %w", err)
	}
	for rows.Next() {
		var x sourceImage
		if err := rows.Scan(&x.ID, &x.ProductID, &x.VirtualIPID, &x.ParentID, &x.URL, &x.Prompt, &x.InputHash, &x.PoseID, &x.MakeupID, &x.AccessoryID, &x.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		m.StyleImages = append(m.StyleImages, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT id, product_id, ip_id, style_image_id, url, COALESCE(prompt,''), input_hash, scene_id, composition, generation_path, created_at FROM first_frames ORDER BY id`)
	if err != nil {
		return fmt.Errorf("first_frames: %w", err)
	}
	for rows.Next() {
		var x sourceImage
		if err := rows.Scan(&x.ID, &x.ProductID, &x.VirtualIPID, &x.ParentID, &x.URL, &x.Prompt, &x.InputHash, &x.SceneID, &x.Composition, &x.GenerationPath, &x.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		m.FirstFrames = append(m.FirstFrames, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT id, video_id, product_id, COALESCE(ip_id,''), scene_id, COALESCE(template_name,''), url, COALESCE(thumbnail,''), COALESCE(title,''), COALESCE(content,''), COALESCE(manual_clip_url,''), status, is_qualified, is_published, created_at, updated_at FROM video_pushes ORDER BY id`)
	if err != nil {
		return fmt.Errorf("video_pushes: %w", err)
	}
	for rows.Next() {
		var p sourceVideoPush
		var csv string
		if err := rows.Scan(&p.ID, &csv, &p.ProductID, &p.VirtualIPID, &p.SceneID, &p.TemplateName, &p.URL, &p.Thumbnail, &p.Title, &p.Content, &p.ManualClipURL, &p.Status, &p.IsQualified, &p.IsPublished, &p.CreatedAt, &p.UpdatedAt); err != nil {
			rows.Close()
			return err
		}
		for _, id := range strings.Split(csv, ",") {
			if id = strings.TrimSpace(id); id != "" {
				p.VideoIDs = append(p.VideoIDs, id)
			}
		}
		m.VideoPushes = append(m.VideoPushes, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	buildLineages(m, onlyTask)
	if onlyTask != "" {
		modelIDs, styleIDs, frameIDs, videoIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, l := range m.Lineages {
			modelIDs[l.ModelImageID] = true
			styleIDs[l.StyleImageID] = true
			frameIDs[l.FirstFrameID] = true
			videoIDs[l.VideoID] = true
		}
		m.ModelImages = filterImages(m.ModelImages, modelIDs)
		m.StyleImages = filterImages(m.StyleImages, styleIDs)
		m.FirstFrames = filterImages(m.FirstFrames, frameIDs)
		pushes := m.VideoPushes[:0]
		for _, p := range m.VideoPushes {
			for _, id := range p.VideoIDs {
				if videoIDs[id] {
					pushes = append(pushes, p)
					break
				}
			}
		}
		m.VideoPushes = pushes
	}
	buildGroups(m, onlyTask)
	return nil
}

func filterImages(images []sourceImage, ids map[string]bool) []sourceImage {
	filtered := images[:0]
	for _, image := range images {
		if ids[image.ID] {
			filtered = append(filtered, image)
		}
	}
	return filtered
}

func buildLineages(m *manifest, onlyTask string) {
	models := map[string]sourceImage{}
	styles := map[string]sourceImage{}
	frames := map[string]sourceImage{}
	videos := map[string]video{}
	for _, x := range m.ModelImages {
		models[x.ID] = x
	}
	for _, x := range m.StyleImages {
		styles[x.ID] = x
	}
	for _, x := range m.FirstFrames {
		frames[x.ID] = x
	}
	for _, t := range m.Tasks {
		for _, v := range t.Videos {
			videos[v.ID] = v
		}
	}
	pushesByVideo := map[string][]string{}
	for _, p := range m.VideoPushes {
		included := onlyTask == ""
		for _, id := range p.VideoIDs {
			if _, ok := videos[id]; ok {
				included = true
				pushesByVideo[id] = append(pushesByVideo[id], p.ID)
			}
		}
		if !included {
			continue
		}
		for _, id := range p.VideoIDs {
			if _, ok := videos[id]; !ok && onlyTask == "" {
				m.Issues = append(m.Issues, lineageIssue{Kind: "video_push_missing_video", SourceID: p.ID, RefID: id})
			}
		}
	}
	for _, t := range m.Tasks {
		if len(t.Videos) == 0 {
			m.Issues = append(m.Issues, lineageIssue{Kind: "task_without_video", SourceID: t.ID})
		}
		for _, v := range t.Videos {
			l := videoLineage{VideoID: v.ID, TaskID: t.ID, ProductID: v.ProductID, VirtualIPID: v.IPID, ModelImageID: v.ModelImageID, StyleImageID: v.StyleImageID, FirstFrameID: v.FirstFrameID, MovementID: v.MovementID, VideoPushIDs: pushesByVideo[v.ID]}
			frame, hasFrame := frames[v.FirstFrameID]
			style, hasStyle := styles[v.StyleImageID]
			model, hasModel := models[v.ModelImageID]
			if !hasFrame {
				m.Issues = append(m.Issues, lineageIssue{Kind: "video_missing_first_frame", SourceID: v.ID, RefID: v.FirstFrameID})
			} else {
				l.SceneID = frame.SceneID
				if frame.ParentID != v.StyleImageID {
					m.Issues = append(m.Issues, lineageIssue{Kind: "video_frame_style_mismatch", SourceID: v.ID, RefID: frame.ParentID})
				}
				if frame.ProductID != "" && (frame.ProductID != v.ProductID || frame.VirtualIPID != v.IPID) {
					m.Issues = append(m.Issues, lineageIssue{Kind: "video_frame_inputs_mismatch", SourceID: v.ID, RefID: frame.ID})
				}
			}
			if !hasStyle {
				m.Issues = append(m.Issues, lineageIssue{Kind: "video_missing_style_image", SourceID: v.ID, RefID: v.StyleImageID})
			} else {
				l.PoseID = style.PoseID
				if style.ParentID != v.ModelImageID {
					m.Issues = append(m.Issues, lineageIssue{Kind: "video_style_model_mismatch", SourceID: v.ID, RefID: style.ParentID})
				}
				if style.ProductID != "" && (style.ProductID != v.ProductID || style.VirtualIPID != v.IPID) {
					m.Issues = append(m.Issues, lineageIssue{Kind: "video_style_inputs_mismatch", SourceID: v.ID, RefID: style.ID})
				}
			}
			if !hasModel {
				m.Issues = append(m.Issues, lineageIssue{Kind: "video_missing_model_image", SourceID: v.ID, RefID: v.ModelImageID})
			} else if model.ProductID != v.ProductID || model.VirtualIPID != v.IPID {
				m.Issues = append(m.Issues, lineageIssue{Kind: "video_model_inputs_mismatch", SourceID: v.ID, RefID: model.ID})
			}
			if hasFrame && hasStyle && hasModel {
				for _, tpl := range m.Templates {
					if tpl.VirtualIPID == v.IPID && tpl.PoseID == l.PoseID && tpl.SceneID == l.SceneID && contains(tpl.MovementIDs, v.MovementID) {
						l.TemplateIDs = append(l.TemplateIDs, tpl.ID)
					}
				}
			}
			m.Lineages = append(m.Lineages, l)
		}
	}
	sort.Slice(m.Issues, func(i, j int) bool {
		if m.Issues[i].Kind == m.Issues[j].Kind {
			return m.Issues[i].SourceID < m.Issues[j].SourceID
		}
		return m.Issues[i].Kind < m.Issues[j].Kind
	})
}

// buildGroups only emits intact source chains. A sample export is deliberately
// a partial projection: its push may reference videos outside --only-task, so
// it never claims that a complete source push is ready for migration.
func buildGroups(m *manifest, onlyTask string) {
	lineages := make(map[string]videoLineage, len(m.Lineages))
	for _, l := range m.Lineages {
		lineages[l.VideoID] = l
	}
	brokenVideos := map[string]string{}
	for _, issue := range m.Issues {
		if strings.HasPrefix(issue.Kind, "video_") && issue.Kind != "video_push_missing_video" {
			brokenVideos[issue.SourceID] = issue.Kind
		}
		if issue.Kind == "task_without_video" {
			m.Excluded = append(m.Excluded, lineageExclusion{Kind: "video_task", SourceID: issue.SourceID, Reason: issue.Kind})
		}
	}
	for id, reason := range brokenVideos {
		m.Excluded = append(m.Excluded, lineageExclusion{Kind: "video", SourceID: id, Reason: reason})
	}
	groupedVideos := map[string]bool{}
	if onlyTask == "" {
		for _, push := range m.VideoPushes {
			if len(push.VideoIDs) == 0 {
				m.Excluded = append(m.Excluded, lineageExclusion{Kind: "video_push", SourceID: push.ID, Reason: "no_video_reference"})
				continue
			}
			seen := map[string]bool{}
			var members []videoLineage
			reason := ""
			for _, id := range push.VideoIDs {
				if seen[id] {
					continue
				}
				seen[id] = true
				l, ok := lineages[id]
				if !ok {
					reason = "missing_video_reference"
					break
				}
				if brokenVideos[id] != "" {
					reason = "broken_video_lineage"
					break
				}
				if l.ProductID != push.ProductID || (push.VirtualIPID != "" && l.VirtualIPID != push.VirtualIPID) || (push.SceneID != "" && l.SceneID != push.SceneID) {
					reason = "push_video_inputs_mismatch"
					break
				}
				if len(members) > 0 && (l.ProductID != members[0].ProductID || l.VirtualIPID != members[0].VirtualIPID || l.FirstFrameID != members[0].FirstFrameID) {
					reason = "mixed_video_lineages"
					break
				}
				members = append(members, l)
			}
			if reason != "" {
				m.Excluded = append(m.Excluded, lineageExclusion{Kind: "video_push", SourceID: push.ID, Reason: reason})
				continue
			}
			ids := make([]string, 0, len(members))
			for _, l := range members {
				ids = append(ids, l.VideoID)
				groupedVideos[l.VideoID] = true
			}
			sort.Strings(ids)
			m.Groups = append(m.Groups, lineageGroup{ID: "push:" + push.ID, Kind: "video_push", VideoPushID: push.ID, ProductID: members[0].ProductID, VirtualIPID: members[0].VirtualIPID, FirstFrameID: members[0].FirstFrameID, VideoIDs: ids, TemplateIDs: commonTemplates(members)})
		}
	}
	// Videos without an intact push remain valid historical outputs. Group all
	// movement branches of one first frame instead of inventing one Job per video.
	partial := map[string]*lineageGroup{}
	partialMembers := map[string][]videoLineage{}
	for _, l := range m.Lineages {
		if brokenVideos[l.VideoID] != "" || groupedVideos[l.VideoID] {
			continue
		}
		key := l.ProductID + ":" + l.VirtualIPID + ":" + l.FirstFrameID
		g := partial[key]
		if g == nil {
			kind := "partial_frame"
			if onlyTask != "" {
				kind = "sample_projection"
			}
			g = &lineageGroup{ID: "frame:" + key, Kind: kind, ProductID: l.ProductID, VirtualIPID: l.VirtualIPID, FirstFrameID: l.FirstFrameID}
			partial[key] = g
		}
		g.VideoIDs = append(g.VideoIDs, l.VideoID)
		partialMembers[key] = append(partialMembers[key], l)
	}
	for key, g := range partial {
		sort.Strings(g.VideoIDs)
		g.TemplateIDs = commonTemplates(partialMembers[key])
		m.Groups = append(m.Groups, *g)
	}
	if onlyTask == "" {
		addImageOnlyGroups(m, brokenVideos)
	}
	sort.Slice(m.Groups, func(i, j int) bool { return m.Groups[i].ID < m.Groups[j].ID })
	sort.Slice(m.Excluded, func(i, j int) bool {
		if m.Excluded[i].Kind != m.Excluded[j].Kind {
			return m.Excluded[i].Kind < m.Excluded[j].Kind
		}
		return m.Excluded[i].SourceID < m.Excluded[j].SourceID
	})
}

// Image-only groups preserve valid intermediate output, without pretending
// that an unfinished chain has a VideoTask or a completed workflow Job.
func addImageOnlyGroups(m *manifest, brokenVideos map[string]string) {
	models := make(map[string]sourceImage, len(m.ModelImages))
	styles := make(map[string]sourceImage, len(m.StyleImages))
	styleChildren := map[string]bool{}
	frameChildren := map[string]bool{}
	videoChildren := map[string]bool{}
	for _, model := range m.ModelImages {
		models[model.ID] = model
	}
	for _, style := range m.StyleImages {
		model, ok := models[style.ParentID]
		if !ok || imageInputsMismatch(style, model) {
			m.Excluded = append(m.Excluded, lineageExclusion{Kind: "style_image", SourceID: style.ID, Reason: "missing_or_mismatched_model_image"})
			continue
		}
		styles[style.ID] = style
		styleChildren[model.ID] = true
	}
	validFrames := map[string]sourceImage{}
	for _, frame := range m.FirstFrames {
		style, ok := styles[frame.ParentID]
		if !ok || imageInputsMismatch(frame, style) {
			m.Excluded = append(m.Excluded, lineageExclusion{Kind: "first_frame", SourceID: frame.ID, Reason: "missing_or_mismatched_image_parent"})
			continue
		}
		validFrames[frame.ID] = frame
		frameChildren[style.ID] = true
	}
	for _, l := range m.Lineages {
		if brokenVideos[l.VideoID] == "" {
			videoChildren[l.FirstFrameID] = true
		}
	}
	for _, model := range m.ModelImages {
		if !styleChildren[model.ID] {
			m.Groups = append(m.Groups, lineageGroup{ID: "model:" + model.ID, Kind: "partial_model_image", ExecutionState: "awaiting_resume", NextStepKey: "style_image", ProductID: model.ProductID, VirtualIPID: model.VirtualIPID, ModelImageID: model.ID})
		}
	}
	for _, style := range m.StyleImages {
		if _, ok := styles[style.ID]; !ok {
			continue
		}
		if !frameChildren[style.ID] {
			m.Groups = append(m.Groups, lineageGroup{ID: "style:" + style.ID, Kind: "partial_style_image", ExecutionState: "awaiting_resume", NextStepKey: "first_frame", ProductID: style.ProductID, VirtualIPID: style.VirtualIPID, ModelImageID: style.ParentID, StyleImageID: style.ID})
		}
	}
	for _, frame := range m.FirstFrames {
		if _, ok := validFrames[frame.ID]; !ok {
			continue
		}
		if !videoChildren[frame.ID] {
			style := styles[frame.ParentID]
			m.Groups = append(m.Groups, lineageGroup{ID: "frame:" + frame.ID, Kind: "partial_first_frame", ExecutionState: "awaiting_resume", NextStepKey: "video", ProductID: frame.ProductID, VirtualIPID: frame.VirtualIPID, ModelImageID: style.ParentID, StyleImageID: style.ID, FirstFrameID: frame.ID})
		}
	}
}

func imageInputsMismatch(child, parent sourceImage) bool {
	return (child.ProductID != "" && child.ProductID != parent.ProductID) || (child.VirtualIPID != "" && child.VirtualIPID != parent.VirtualIPID)
}

func commonTemplates(members []videoLineage) []string {
	if len(members) == 0 {
		return nil
	}
	common := append([]string(nil), members[0].TemplateIDs...)
	for _, l := range members[1:] {
		filtered := common[:0]
		for _, id := range common {
			if contains(l.TemplateIDs, id) {
				filtered = append(filtered, id)
			}
		}
		common = filtered
	}
	sort.Strings(common)
	return common
}

func contains(values []string, value string) bool {
	for _, x := range values {
		if x == value {
			return true
		}
	}
	return false
}

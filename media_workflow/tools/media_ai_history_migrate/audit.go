package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// readinessAudit is a read-only migration gate, not an import plan. IDs in
// Missing are source IDs; no API token, signed URL or source URL is emitted.
type readinessAudit struct {
	Groups              int                 `json:"groups"`
	ReadyGroups         int                 `json:"ready_groups"`
	MediaBlockedGroups  []string            `json:"media_blocked_groups,omitempty"`
	MediaBlockedPushes  []string            `json:"media_blocked_pushes,omitempty"`
	Required            map[string]int      `json:"required"`
	Mapped              map[string]int      `json:"mapped"`
	Missing             map[string][]string `json:"missing"`
	TargetObjectMissing map[string][]string `json:"target_object_missing,omitempty"`
	MediaChecked        int                 `json:"media_checked"`
	MediaOK             int                 `json:"media_ok"`
	MediaFailed         []mediaFailure      `json:"media_failed,omitempty"`
}

type mediaFailure struct {
	Kind   string `json:"kind"`
	ID     string `json:"source_id"`
	Reason string `json:"reason"`
}

type auditTarget struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	AssetKind      string          `json:"asset_kind"`
	OrganizationID string          `json:"organization_id"`
	Scope          string          `json:"scope"`
	Metadata       json.RawMessage `json:"metadata"`
	ObjectPath     string          `json:"object_path"`
}

type auditMedia struct{ kind, id, rawURL string }

func auditMigrationReadiness(ctx context.Context, db *sql.DB, m manifest, cfg importConfig) error {
	if cfg.BearerToken == "" || cfg.OrganizationID == "" {
		return fmt.Errorf("--audit-readiness requires organization ID and User API Token")
	}
	if err := validateOrigin("target-gateway", cfg.GatewayURL); err != nil {
		return err
	}
	if err := validateOrigin("source-assets-url", cfg.SourceAssetsURL); err != nil {
		return err
	}
	sourceProducts, err := sourceAuditIDs(ctx, db, "SELECT id FROM products")
	if err != nil {
		return err
	}
	sourceVIPs, err := sourceAuditPairs(ctx, db, "SELECT id, nickname FROM virtual_ips")
	if err != nil {
		return err
	}
	sourceMaterials, err := sourceAuditTriples(ctx, db, "SELECT id, type, COALESCE(url,'') FROM materials")
	if err != nil {
		return err
	}
	sourceMovements, err := sourceAuditPairs(ctx, db, "SELECT id, COALESCE(url,'') FROM movement_materials")
	if err != nil {
		return err
	}
	products, err := listAuditTargets(ctx, cfg, "products")
	if err != nil {
		return err
	}
	vips, err := listAuditTargets(ctx, cfg, "virtual-ips")
	if err != nil {
		return err
	}
	materials, err := listAuditTargets(ctx, cfg, "materials")
	if err != nil {
		return err
	}
	required := map[string]map[string]bool{}
	for _, kind := range []string{"product", "virtual_ip", "pose", "scene", "movement", "model_image", "style_image", "first_frame", "video"} {
		required[kind] = map[string]bool{}
	}
	styleByID, frameByID, modelByID, videoByID := map[string]sourceImage{}, map[string]sourceImage{}, map[string]sourceImage{}, map[string]video{}
	for _, image := range m.ModelImages {
		modelByID[image.ID] = image
	}
	for _, image := range m.StyleImages {
		styleByID[image.ID] = image
	}
	for _, image := range m.FirstFrames {
		frameByID[image.ID] = image
	}
	for _, task := range m.Tasks {
		for _, item := range task.Videos {
			videoByID[item.ID] = item
		}
	}
	for _, group := range m.Groups {
		markRequired(required, "product", group.ProductID)
		markRequired(required, "virtual_ip", group.VirtualIPID)
		markRequired(required, "model_image", group.ModelImageID)
		markRequired(required, "style_image", group.StyleImageID)
		markRequired(required, "first_frame", group.FirstFrameID)
		if style, ok := styleByID[group.StyleImageID]; ok {
			markRequired(required, "pose", style.PoseID)
		}
		if frame, ok := frameByID[group.FirstFrameID]; ok {
			markRequired(required, "scene", frame.SceneID)
		}
		for _, id := range group.VideoIDs {
			markRequired(required, "video", id)
			if item, ok := videoByID[id]; ok {
				markRequired(required, "movement", item.MovementID)
			}
		}
	}
	productTargets := map[string]int{}
	for _, item := range products {
		if id := auditSourceID(item.Metadata); id != "" {
			productTargets[id]++
		}
	}
	vipNames := map[string]int{}
	for _, item := range vips {
		vipNames[item.Name]++
	}
	materialTargets := map[string]map[string]int{"pose": {}, "scene": {}, "movement": {}}
	targetObjectMissing := map[string][]string{}
	for _, item := range materials {
		kind := strings.ToLower(item.AssetKind)
		if bucket := materialTargets[kind]; bucket != nil {
			if id := auditSourceID(item.Metadata); id != "" {
				bucket[id]++
				if required[kind][id] && item.ObjectPath == "" {
					targetObjectMissing[kind] = append(targetObjectMissing[kind], id)
				}
			}
		}
	}
	for kind := range targetObjectMissing {
		sort.Strings(targetObjectMissing[kind])
	}
	report := readinessAudit{Groups: len(m.Groups), Required: map[string]int{}, Mapped: map[string]int{}, Missing: map[string][]string{}, TargetObjectMissing: targetObjectMissing}
	media := make([]auditMedia, 0)
	intactPushes := map[string]bool{}
	for _, group := range m.Groups {
		if group.Kind == "video_push" {
			intactPushes[group.VideoPushID] = true
		}
	}
	for _, push := range m.VideoPushes {
		if !intactPushes[push.ID] {
			continue
		}
		for _, item := range []struct{ kind, rawURL string }{
			{"video_push_video", push.URL},
			{"video_push_thumbnail", push.Thumbnail},
			{"video_push_manual_clip", push.ManualClipURL},
		} {
			if item.rawURL != "" {
				media = append(media, auditMedia{item.kind, push.ID, item.rawURL})
			}
		}
	}
	for kind, ids := range required {
		report.Required[kind] = len(ids)
		for id := range ids {
			found := false
			switch kind {
			case "product":
				found = sourceProducts[id] && productTargets[id] == 1
			case "virtual_ip":
				found = sourceVIPs[id] != "" && vipNames[sourceVIPs[id]] == 1
			case "pose", "scene":
				value, exists := sourceMaterials[id]
				found = exists && strings.EqualFold(value[0], kind) && materialTargets[kind][id] == 1
				if exists {
					media = append(media, auditMedia{kind, id, value[1]})
				}
			case "movement":
				value, exists := sourceMovements[id]
				found = exists && materialTargets[kind][id] == 1
				if exists {
					media = append(media, auditMedia{kind, id, value})
				}
			case "model_image":
				item, exists := modelByID[id]
				found = exists
				if exists {
					media = append(media, auditMedia{kind, id, item.URL})
				}
			case "style_image":
				item, exists := styleByID[id]
				found = exists
				if exists {
					media = append(media, auditMedia{kind, id, item.URL})
				}
			case "first_frame":
				item, exists := frameByID[id]
				found = exists
				if exists {
					media = append(media, auditMedia{kind, id, item.URL})
				}
			case "video":
				item, exists := videoByID[id]
				found = exists
				if exists {
					media = append(media, auditMedia{kind, id, item.URL})
				}
			}
			if found {
				report.Mapped[kind]++
			} else {
				report.Missing[kind] = append(report.Missing[kind], id)
			}
		}
		sort.Strings(report.Missing[kind])
	}
	probeAuditMedia(ctx, cfg.SourceAssetsURL, media, &report)
	failedMedia := map[string]bool{}
	for _, item := range report.MediaFailed {
		failedMedia[item.Kind+":"+item.ID] = true
	}
	for _, push := range m.VideoPushes {
		if intactPushes[push.ID] && (failedMedia["video_push_video:"+push.ID] || failedMedia["video_push_thumbnail:"+push.ID] || failedMedia["video_push_manual_clip:"+push.ID]) {
			report.MediaBlockedPushes = append(report.MediaBlockedPushes, push.ID)
		}
	}
	sort.Strings(report.MediaBlockedPushes)
	for _, group := range m.Groups {
		blocked := failedMedia["model_image:"+group.ModelImageID] || failedMedia["style_image:"+group.StyleImageID] || failedMedia["first_frame:"+group.FirstFrameID]
		if style, ok := styleByID[group.StyleImageID]; ok {
			blocked = blocked || failedMedia["pose:"+style.PoseID]
		}
		if frame, ok := frameByID[group.FirstFrameID]; ok {
			blocked = blocked || failedMedia["scene:"+frame.SceneID]
		}
		for _, id := range group.VideoIDs {
			blocked = blocked || failedMedia["video:"+id]
			if item, ok := videoByID[id]; ok {
				blocked = blocked || failedMedia["movement:"+item.MovementID]
			}
		}
		if blocked {
			report.MediaBlockedGroups = append(report.MediaBlockedGroups, group.ID)
		}
	}
	sort.Strings(report.MediaBlockedGroups)
	report.ReadyGroups = report.Groups - len(report.MediaBlockedGroups)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf("readiness audit (read-only): %s\n", encoded)
	return nil
}

func markRequired(required map[string]map[string]bool, kind, id string) {
	if id != "" {
		required[kind][id] = true
	}
}

func auditSourceID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var encoded string
	if raw[0] == '"' && json.Unmarshal(raw, &encoded) == nil {
		raw = []byte(encoded)
	}
	var value struct {
		Migration struct {
			Source string `json:"source"`
			ID     string `json:"source_id"`
		} `json:"migration"`
	}
	if json.Unmarshal(raw, &value) != nil || value.Migration.Source != "media_ai" {
		return ""
	}
	return value.Migration.ID
}

func listAuditTargets(ctx context.Context, cfg importConfig, resource string) ([]auditTarget, error) {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	all := []auditTarget{}
	for page := 1; ; page++ {
		query := url.Values{"organization_id": {cfg.OrganizationID}, "scope": {"ai_clothing"}, "page": {fmt.Sprint(page)}, "page_size": {"100"}}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.GatewayURL+"/api/v1/"+resource+"?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+cfg.BearerToken)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list target %s: %w", resource, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("list target %s: HTTP %d", resource, resp.StatusCode)
		}
		var result struct {
			Items []auditTarget `json:"items"`
			Total int           `json:"total"`
		}
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			if item.OrganizationID != cfg.OrganizationID || item.Scope != "ai_clothing" {
				return nil, fmt.Errorf("target %s returned a foreign item", resource)
			}
		}
		all = append(all, result.Items...)
		if len(result.Items) == 0 || len(all) >= result.Total {
			return all, nil
		}
	}
}

func sourceAuditIDs(ctx context.Context, db *sql.DB, query string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		items[id] = true
	}
	return items, rows.Err()
}

func sourceAuditPairs(ctx context.Context, db *sql.DB, query string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[string]string{}
	for rows.Next() {
		var id, value string
		if err := rows.Scan(&id, &value); err != nil {
			return nil, err
		}
		items[id] = value
	}
	return items, rows.Err()
}

func sourceAuditTriples(ctx context.Context, db *sql.DB, query string) (map[string][2]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[string][2]string{}
	for rows.Next() {
		var id, kind, value string
		if err := rows.Scan(&id, &kind, &value); err != nil {
			return nil, err
		}
		items[id] = [2]string{kind, value}
	}
	return items, rows.Err()
}

func probeAuditMedia(ctx context.Context, origin string, media []auditMedia, report *readinessAudit) {
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tasks := make(chan auditMedia)
	results := make(chan mediaFailure, len(media))
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range tasks {
				reason := probeOneAuditMedia(ctx, client, origin, item.rawURL)
				results <- mediaFailure{Kind: item.kind, ID: item.id, Reason: reason}
			}
		}()
	}
	for _, item := range media {
		tasks <- item
	}
	close(tasks)
	workers.Wait()
	close(results)
	for result := range results {
		report.MediaChecked++
		if result.Reason == "" {
			report.MediaOK++
		} else {
			report.MediaFailed = append(report.MediaFailed, result)
		}
	}
	sort.Slice(report.MediaFailed, func(i, j int) bool {
		if report.MediaFailed[i].Kind == report.MediaFailed[j].Kind {
			return report.MediaFailed[i].ID < report.MediaFailed[j].ID
		}
		return report.MediaFailed[i].Kind < report.MediaFailed[j].Kind
	})
}

func probeOneAuditMedia(ctx context.Context, client *http.Client, origin, raw string) string {
	if raw == "" {
		return "missing URL"
	}
	resolved, err := resolveSourceMediaURL(origin, raw)
	if err != nil {
		return "invalid or foreign URL"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved, nil)
	if err != nil {
		return "invalid request"
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := client.Do(req)
	if err != nil {
		return "request failed"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return "HTML instead of media"
	}
	if _, err := io.CopyN(io.Discard, resp.Body, 1); err != nil {
		return "empty or unreadable body"
	}
	return ""
}

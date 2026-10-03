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
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const (
	maxImportMediaBytes int64 = 256 << 20
	maxImportImageBytes int64 = 16 << 20
)

type importConfig struct {
	SourceAssetsURL string
	GatewayURL      string
	MaterialsURL    string
	OrganizationID  string
	OwnerUserID     string
	ProductID       string
	VirtualIPID     string
	BearerToken     string
	MigrationToken  string
	WorkflowURL     string
	WorkflowToken   string
	WorkflowID      string
	WorkflowVersion int
	WorkspaceID     string
	ImportJob       bool
	OnImportedMedia func(mediaArtifact)
}

type mediaArtifact struct {
	Kind       string `json:"kind"`
	SourceID   string `json:"source_id"`
	TargetID   string `json:"target_id,omitempty"`
	ObjectPath string `json:"object_path"`
	SHA256     string `json:"sha256"`
}

type historyImporter struct {
	cfg    importConfig
	client *http.Client
}

func newHistoryImporter(cfg importConfig) historyImporter {
	return historyImporter{cfg: cfg, client: &http.Client{
		Timeout: 10 * time.Minute,
		// Source and target origins are explicitly configured. A redirect may
		// escape those origins or silently switch the API authentication path.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func importImageGroup(ctx context.Context, db *sql.DB, m *manifest, groupID string, cfg importConfig) error {
	_, err := importImageGroupID(ctx, db, m, groupID, cfg)
	return err
}

func importImageGroupID(ctx context.Context, db *sql.DB, m *manifest, groupID string, cfg importConfig) (string, error) {
	if err := validateImportConfig(cfg); err != nil {
		return "", err
	}
	var group *lineageGroup
	for i := range m.Groups {
		if m.Groups[i].ID == groupID {
			group = &m.Groups[i]
			break
		}
	}
	if group == nil {
		return "", fmt.Errorf("lineage group %q not found", groupID)
	}
	if cfg.ImportJob {
		if cfg.WorkflowURL == "" || cfg.WorkflowToken == "" || cfg.WorkflowID == "" || cfg.WorkflowVersion <= 0 || cfg.WorkspaceID == "" {
			return "", errors.New("--import-job requires target workflow URL, migration token, published workflow ID/version, and target Workspace ID")
		}
		if err := validateOrigin("workflow-internal-url", cfg.WorkflowURL); err != nil {
			return "", err
		}
	}
	if group.ExecutionState != "awaiting_resume" || group.Kind != "partial_model_image" && group.Kind != "partial_style_image" && group.Kind != "partial_first_frame" {
		return "", fmt.Errorf("group %q is not an intact unfinished image workflow", groupID)
	}
	importer := newHistoryImporter(cfg)
	if err := importer.verifyInputMappings(ctx, db, *group); err != nil {
		return "", err
	}
	model, ok := findSourceImage(m.ModelImages, group.ModelImageID)
	if !ok {
		return "", fmt.Errorf("model image %q missing from source group", group.ModelImageID)
	}
	if model.ProductID != group.ProductID || model.VirtualIPID != group.VirtualIPID {
		return "", errors.New("model image product or virtual IP differs from source group")
	}
	images := []struct {
		kind  string
		image sourceImage
	}{{"model_image", model}}
	if group.StyleImageID != "" {
		style, found := findSourceImage(m.StyleImages, group.StyleImageID)
		if !found || style.ParentID != model.ID || style.ProductID != group.ProductID || style.VirtualIPID != group.VirtualIPID {
			return "", errors.New("source group style image is disconnected")
		}
		images = append(images, struct {
			kind  string
			image sourceImage
		}{"style_image", style})
	}
	if group.FirstFrameID != "" {
		frame, found := findSourceImage(m.FirstFrames, group.FirstFrameID)
		if !found || frame.ParentID != group.StyleImageID || frame.ProductID != group.ProductID || frame.VirtualIPID != group.VirtualIPID {
			return "", errors.New("source group first frame is disconnected")
		}
		images = append(images, struct {
			kind  string
			image sourceImage
		}{"first_frame", frame})
	}
	completed := make([]map[string]string, 0, len(images))
	for _, item := range images {
		id, err := importer.importImage(ctx, item.kind, item.image)
		if err != nil {
			return "", fmt.Errorf("%s %s: %w", item.kind, item.image.ID, err)
		}
		fmt.Printf("imported historical asset: kind=%s source_id=%s target_id=%s\n", item.kind, item.image.ID, id)
		completed = append(completed, map[string]string{"step_key": item.kind, "source_record_id": item.image.ID, "asset_id": id})
	}
	if cfg.ImportJob {
		jobID, err := importer.importIncompleteJob(ctx, group.ID, completed)
		if err != nil {
			return "", fmt.Errorf("historical assets were imported, but Job import failed: %w", err)
		}
		fmt.Printf("confirmed historical Job: group=%s job_id=%s source_next_step=%s\n", group.ID, jobID, group.NextStepKey)
		return jobID, nil
	} else {
		fmt.Printf("image group %s assets imported; pass --import-job after Workspace/Workflow mapping, next_step=%s\n", group.ID, group.NextStepKey)
	}
	return "", nil
}

func validateImportConfig(cfg importConfig) error {
	if cfg.OrganizationID == "" || cfg.OwnerUserID == "" || cfg.ProductID == "" || cfg.VirtualIPID == "" || cfg.BearerToken == "" || cfg.MigrationToken == "" {
		return errors.New("--import-group requires organization, owner, product, virtual IP, API token, and migration token")
	}
	for label, raw := range map[string]string{"source-assets-url": cfg.SourceAssetsURL, "target-gateway": cfg.GatewayURL, "materials-internal-url": cfg.MaterialsURL} {
		if err := validateOrigin(label, raw); err != nil {
			return err
		}
	}
	return nil
}

func validateOrigin(label, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid %s origin", label)
	}
	return nil
}

func (h historyImporter) importIncompleteJob(ctx context.Context, groupID string, completed []map[string]string) (string, error) {
	return h.importHistoricalJob(ctx, "/api/internal/v1/jobs/import-incomplete", groupID, completed, 7)
}

func (h historyImporter) importHistoricalJob(ctx context.Context, endpoint, groupID string, completed []map[string]string, expectedStatus int) (string, error) {
	request := map[string]any{
		"source_group_id": groupID, "organization_id": h.cfg.OrganizationID, "workspace_id": h.cfg.WorkspaceID,
		"owner_user_id": h.cfg.OwnerUserID, "workflow_id": h.cfg.WorkflowID, "workflow_version": h.cfg.WorkflowVersion,
		"input":     map[string]any{"product": map[string]string{"id": h.cfg.ProductID}, "virtual_ip": map[string]string{"id": h.cfg.VirtualIPID}},
		"completed": completed,
	}
	encoded, _ := json.Marshal(request)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.WorkflowURL+endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-Workflow-Migration-Token", h.cfg.WorkflowToken)
	var result struct {
		JobID  string `json:"job_id"`
		Status int    `json:"status"`
	}
	if err := h.do(req, &result); err != nil {
		return "", err
	}
	statusValid := result.Status == expectedStatus
	// Replayed incomplete imports return the existing Job's current state,
	// including a Job the user already resumed and completed.
	if expectedStatus == 7 {
		statusValid = result.Status >= 1 && result.Status <= 7
	}
	if result.JobID == "" || !statusValid {
		return "", fmt.Errorf("target did not confirm historical Job status %d", expectedStatus)
	}
	return result.JobID, nil
}

func findSourceImage(images []sourceImage, id string) (sourceImage, bool) {
	for _, image := range images {
		if image.ID == id {
			return image, true
		}
	}
	return sourceImage{}, false
}

func (h historyImporter) verifyInputMappings(ctx context.Context, db *sql.DB, group lineageGroup) error {
	var sourceVIPName string
	if err := db.QueryRowContext(ctx, "SELECT nickname FROM virtual_ips WHERE id = ?", group.VirtualIPID).Scan(&sourceVIPName); err != nil {
		return fmt.Errorf("source virtual IP mapping: %w", err)
	}
	var product struct {
		ID             string          `json:"id"`
		OrganizationID string          `json:"organization_id"`
		Scope          string          `json:"scope"`
		Metadata       json.RawMessage `json:"metadata"`
	}
	if err := h.getTarget(ctx, "/api/v1/products/"+url.PathEscape(h.cfg.ProductID)+"?organization_id="+url.QueryEscape(h.cfg.OrganizationID), &product); err != nil {
		return fmt.Errorf("target product mapping: %w", err)
	}
	var metadata struct {
		Migration struct {
			SourceID string `json:"source_id"`
		} `json:"migration"`
	}
	if len(product.Metadata) > 0 && product.Metadata[0] == '"' {
		var encoded string
		if err := json.Unmarshal(product.Metadata, &encoded); err != nil {
			return err
		}
		product.Metadata = []byte(encoded)
	}
	if err := json.Unmarshal(product.Metadata, &metadata); err != nil || metadata.Migration.SourceID != group.ProductID || product.OrganizationID != h.cfg.OrganizationID || product.Scope != "ai_clothing" {
		return errors.New("target product does not map to source product in organization/scope")
	}
	var vip struct {
		ID             string `json:"id"`
		OrganizationID string `json:"organization_id"`
		Scope          string `json:"scope"`
		Name           string `json:"name"`
	}
	if err := h.getTarget(ctx, "/api/v1/virtual-ips/"+url.PathEscape(h.cfg.VirtualIPID)+"?organization_id="+url.QueryEscape(h.cfg.OrganizationID), &vip); err != nil {
		return fmt.Errorf("target virtual IP mapping: %w", err)
	}
	if vip.Name != sourceVIPName || vip.OrganizationID != h.cfg.OrganizationID || vip.Scope != "ai_clothing" {
		return errors.New("target virtual IP does not map to source virtual IP in organization/scope")
	}
	return nil
}

func (h historyImporter) getTarget(ctx context.Context, endpoint string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.cfg.GatewayURL+endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.cfg.BearerToken)
	return h.do(req, result)
}

func (h historyImporter) importImage(ctx context.Context, kind string, image sourceImage) (string, error) {
	return h.importMedia(ctx, kind, image.ID, image.URL, image.CreatedAt, "image/")
}

func (h historyImporter) importMedia(ctx context.Context, kind, sourceID, rawURL string, createdAt time.Time, mediaPrefix string) (string, error) {
	if createdAt.IsZero() || sourceID == "" {
		return "", errors.New("source media identity or creation time is missing")
	}
	sourceURL, err := resolveSourceMediaURL(h.cfg.SourceAssetsURL, rawURL)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("source media HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "media-ai-source-*")
	if err != nil {
		return "", fmt.Errorf("create source media spool: %w", err)
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(resp.Body, maxImportMediaBytes+1))
	if err != nil || size == 0 || size > maxImportMediaBytes {
		return "", errors.New("source media unreadable, empty, or exceeds 256 MiB")
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind source media: %w", err)
	}
	header := make([]byte, 512)
	headerSize, err := io.ReadFull(tmp, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("inspect source media: %w", err)
	}
	mediaType := detectSourceMediaType(header[:headerSize])
	if !strings.HasPrefix(mediaType, mediaPrefix) {
		return "", fmt.Errorf("%s source media type %s does not match %s", kind, mediaType, mediaPrefix)
	}
	if mediaPrefix == "image/" && size > maxImportImageBytes {
		return "", errors.New("source image exceeds 16 MiB")
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind source media: %w", err)
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	objectPath, err := h.upload(ctx, kind, sourceID, path.Base(rawURL), tmp, checksum)
	if err != nil {
		return "", err
	}
	request := map[string]any{
		"organization_id": h.cfg.OrganizationID, "scope": "ai_clothing", "asset_kind": kind,
		"name": "media_ai:" + kind + ":" + sourceID, "object_path": objectPath,
		"owner_user_id": h.cfg.OwnerUserID, "source_system": "media_ai", "source_record_id": sourceID,
		"source_sha256": checksum, "source_created_at": createdAt.UnixNano(),
		"source_virtual_ip_id": h.cfg.VirtualIPID, "source_product_id": h.cfg.ProductID,
	}
	encoded, _ := json.Marshal(request)
	importReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.MaterialsURL+"/api/internal/v1/generated-assets/import", bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	importReq.Header.Set("Content-Type", "application/json")
	importReq.Header.Set("X-Media-Materials-Migration-Token", h.cfg.MigrationToken)
	var result struct {
		ID         string `json:"id"`
		ObjectPath string `json:"object_path"`
	}
	if err := h.do(importReq, &result); err != nil {
		return "", err
	}
	if result.ID == "" || result.ObjectPath != objectPath {
		return "", errors.New("target import response did not confirm object")
	}
	if h.cfg.OnImportedMedia != nil {
		h.cfg.OnImportedMedia(mediaArtifact{Kind: kind, SourceID: sourceID, TargetID: result.ID, ObjectPath: objectPath, SHA256: checksum})
	}
	return result.ID, nil
}

func detectSourceMediaType(payload []byte) string {
	if len(payload) >= 12 && string(payload[4:8]) == "ftyp" {
		return "video/mp4"
	}
	if len(payload) >= 4 && payload[0] == 0x1a && payload[1] == 0x45 && payload[2] == 0xdf && payload[3] == 0xa3 {
		return "video/webm"
	}
	return http.DetectContentType(payload)
}

func resolveSourceMediaURL(origin, raw string) (string, error) {
	base, err := url.Parse(origin)
	if err != nil || base.Host == "" || base.User != nil || base.Path != "" || base.RawQuery != "" || base.Fragment != "" || base.Scheme != "http" && base.Scheme != "https" {
		return "", errors.New("invalid source assets origin")
	}
	ref, err := url.Parse(raw)
	if err != nil || ref.User != nil || ref.RawQuery != "" || ref.Fragment != "" || !strings.HasPrefix(ref.Path, "/uploads/") || path.Clean(ref.Path) != ref.Path {
		return "", errors.New("source media URL is not a clean /uploads path")
	}
	if ref.IsAbs() && (ref.Scheme != base.Scheme || ref.Host != base.Host) {
		return "", errors.New("source media URL leaves configured origin")
	}
	if ref.Host != "" && !ref.IsAbs() {
		return "", errors.New("scheme-relative source media URL is forbidden")
	}
	return base.ResolveReference(ref).String(), nil
}

func (h historyImporter) upload(ctx context.Context, kind, sourceID, filename string, source *os.File, checksum string) (string, error) {
	return h.uploadWithPurpose(ctx, "generated_asset_import", kind, sourceID, filename, source, checksum)
}

func (h historyImporter) uploadWithPurpose(ctx context.Context, purpose, kind, sourceID, filename string, source *os.File, checksum string) (string, error) {
	body, err := os.CreateTemp("", "media-ai-multipart-*")
	if err != nil {
		return "", fmt.Errorf("create multipart spool: %w", err)
	}
	defer func() { _ = body.Close(); _ = os.Remove(body.Name()) }()
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, source); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	length, err := body.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	query := url.Values{"organization_id": {h.cfg.OrganizationID}, "scope": {"ai_clothing"}, "purpose": {purpose}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.GatewayURL+"/api/v1/library-objects:upload?"+query.Encode(), body)
	if err != nil {
		return "", err
	}
	req.ContentLength = length
	req.Header.Set("Authorization", "Bearer "+h.cfg.BearerToken)
	idempotencyKind := strings.ReplaceAll(kind, "_", "-")
	req.Header.Set("Idempotency-Key", "media-ai-"+idempotencyKind+"-"+sourceID)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	var result struct {
		ObjectPath string `json:"object_path"`
		Checksum   string `json:"checksum"`
	}
	if err := h.do(req, &result); err != nil {
		return "", err
	}
	if result.Checksum != checksum || result.ObjectPath == "" {
		return "", errors.New("target upload checksum mismatch")
	}
	return result.ObjectPath, nil
}

func (h historyImporter) do(req *http.Request, result any) error {
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	encoded, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(encoded)))
	}
	if err := json.Unmarshal(encoded, result); err != nil {
		return fmt.Errorf("decode %s response: %w", req.URL.Path, err)
	}
	return nil
}

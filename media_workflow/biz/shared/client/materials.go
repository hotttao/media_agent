package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"media_agent/media_workflow/biz/shared"
)

// MaterialsClient 调 media_materials 的内部 RPC（HTTP）。
// 实现 domain.AssetKindLookup 端口。main.go 注入。
type MaterialsClient struct {
	baseURL string
	http    *http.Client
}

var materialsTracer = otel.Tracer("media-workflow/media-materials-client")

// startDownstreamRequest creates a child span and injects W3C trace context so
// the downstream service can join the same trace. With OTel disabled this is a
// no-op tracer, while the propagation contract remains safe and deterministic.
func startDownstreamRequest(ctx context.Context, req *http.Request, operation string) (context.Context, trace.Span) {
	ctx, span := materialsTracer.Start(ctx, operation)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	return ctx, span
}

func NewMaterialsClient(baseURL string) *MaterialsClient {
	return &MaterialsClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

type batchGetReq struct {
	Names []string `json:"names"`
}

type assetKindDTO struct {
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
	MediaType    string `json:"media_type"`
	Status       string `json:"status"`
}

type batchGetResp struct {
	Kinds    []assetKindDTO `json:"kinds"`
	NotFound []string       `json:"not_found"`
}

// BatchGet 实现 AssetKindLookup。返回 (name→info, not_found, err)。
func (c *MaterialsClient) BatchGet(ctx context.Context, names []string) (map[string]*shared.AssetKindInfo, []string, error) {
	if len(names) == 0 {
		return map[string]*shared.AssetKindInfo{}, nil, nil
	}
	body, err := json.Marshal(batchGetReq{Names: names})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/internal/v1/asset-kinds:batchGet", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	ctx, span := startDownstreamRequest(ctx, req, "media_materials.BatchGetAssetKind")
	defer span.End()
	req = req.WithContext(ctx)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("call materials BatchGetAssetKind: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("materials BatchGetAssetKind: status %d", resp.StatusCode)
	}
	var out batchGetResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, nil, fmt.Errorf("decode BatchGetAssetKind resp: %w", err)
	}
	m := make(map[string]*shared.AssetKindInfo, len(out.Kinds))
	for _, k := range out.Kinds {
		m[k.Name] = &shared.AssetKindInfo{
			Name: k.Name, ResourceType: k.ResourceType, MediaType: k.MediaType, Status: k.Status,
		}
	}
	return m, out.NotFound, nil
}

// UpsertGenAssetReq 落产物请求。
type UpsertGenAssetReq struct {
	Scope             string `json:"scope"`
	AssetKind         string `json:"asset_kind"`
	Name              string `json:"name"`
	URL               string `json:"url"`
	Content           string `json:"content"`
	OwnerUserID       string `json:"owner_user_id"`
	Visibility        string `json:"visibility"`
	JobID             string `json:"job_id"`
	StepRunID         string `json:"step_run_id"`
	ResultKey         string `json:"result_key,omitempty"`
	TemplateID        string `json:"template_id"`
	TemplateVersion   int    `json:"template_version"`
	SourceVirtualIPID string `json:"source_virtual_ip_id"`
	SourceProductID   string `json:"source_product_id"`
}

// UpsertGeneratedAsset 调 media_materials 落生成产物。返回 generated_asset id。
func (c *MaterialsClient) UpsertGeneratedAsset(ctx context.Context, req *UpsertGenAssetReq) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/internal/v1/generated-assets:upsert", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Media-Materials-Internal-Token", os.Getenv("MEDIA_MATERIALS_INTERNAL_TOKEN"))
	ctx, span := startDownstreamRequest(ctx, httpReq, "media_materials.UpsertGeneratedAsset")
	defer span.End()
	httpReq = httpReq.WithContext(ctx)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", errors.New("materials generated asset registration unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("materials UpsertGeneratedAsset: status %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", errors.New("materials generated asset response invalid")
	}
	if strings.TrimSpace(out.ID) == "" {
		return "", errors.New("materials generated asset response missing identity")
	}
	return out.ID, nil
}

type ImportedGeneratedAsset struct {
	ID                string `json:"id"`
	OrganizationID    string `json:"organization_id"`
	Scope             string `json:"scope"`
	OwnerUserID       string `json:"owner_user_id"`
	AssetKind         string `json:"asset_kind"`
	ObjectPath        string `json:"object_path"`
	SourceSystem      string `json:"source_system"`
	SourceRecordID    string `json:"source_record_id"`
	SourceSHA256      string `json:"source_sha256"`
	SourceCreatedAt   int64  `json:"source_created_at"`
	SourceProductID   string `json:"source_product_id"`
	SourceVirtualIPID string `json:"source_virtual_ip_id"`
	AccessURL         string `json:"access_url"`
	AccessExpiresAt   int64  `json:"access_expires_at"`
}

type ResolvedStorageObject struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
}

// ResolveInternalObject exchanges a stable, organization-scoped Object Path
// for a short-lived storage URL. Caller authorization must have happened before
// this service-to-service storage lookup.
func (c *MaterialsClient) ResolveInternalObject(ctx context.Context, organizationID, objectPath string) (ResolvedStorageObject, error) {
	if organizationID == "" || objectPath == "" || os.Getenv("MEDIA_MATERIALS_INTERNAL_TOKEN") == "" {
		return ResolvedStorageObject{}, fmt.Errorf("object lookup requires organization, object path and internal token")
	}
	query := url.Values{"organization_id": {organizationID}, "object_path": {objectPath}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/internal/v1/library-objects:resolve?"+query.Encode(), nil)
	if err != nil {
		return ResolvedStorageObject{}, err
	}
	req.Header.Set("X-Media-Materials-Internal-Token", os.Getenv("MEDIA_MATERIALS_INTERNAL_TOKEN"))
	ctx, span := startDownstreamRequest(ctx, req, "media_materials.ResolveInternalObject")
	defer span.End()
	resp, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return ResolvedStorageObject{}, fmt.Errorf("resolve object: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ResolvedStorageObject{}, fmt.Errorf("resolve object: HTTP %d", resp.StatusCode)
	}
	var resolved ResolvedStorageObject
	if err := json.NewDecoder(resp.Body).Decode(&resolved); err != nil {
		return ResolvedStorageObject{}, fmt.Errorf("decode resolved object: %w", err)
	}
	if resolved.URL == "" || resolved.ExpiresAt <= time.Now().Unix() {
		return ResolvedStorageObject{}, fmt.Errorf("invalid resolved object response")
	}
	return resolved, nil
}

func (c *MaterialsClient) GetImportedGeneratedAsset(ctx context.Context, id string) (*ImportedGeneratedAsset, error) {
	if id == "" || os.Getenv("MEDIA_MATERIALS_INTERNAL_TOKEN") == "" {
		return nil, fmt.Errorf("imported asset lookup requires an ID and internal token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/internal/v1/generated-assets/import/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Media-Materials-Internal-Token", os.Getenv("MEDIA_MATERIALS_INTERNAL_TOKEN"))
	ctx, span := startDownstreamRequest(ctx, req, "media_materials.GetImportedGeneratedAsset")
	defer span.End()
	resp, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get imported generated asset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get imported generated asset: HTTP %d", resp.StatusCode)
	}
	var asset ImportedGeneratedAsset
	if err := json.NewDecoder(resp.Body).Decode(&asset); err != nil {
		return nil, fmt.Errorf("decode imported generated asset: %w", err)
	}
	if asset.ID != id || asset.SourceSystem == "" || asset.SourceRecordID == "" {
		return nil, fmt.Errorf("invalid imported generated asset response")
	}
	return &asset, nil
}

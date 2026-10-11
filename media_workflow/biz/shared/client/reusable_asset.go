package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
)

var ErrReusableAssetNotFound = errors.New("reusable asset not found")

type ReusableGeneratedAsset struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	Scope           string `json:"scope"`
	OwnerUserID     string `json:"owner_user_id"`
	AssetKind       string `json:"asset_kind"`
	ObjectPath      string `json:"object_path"`
	Content         string `json:"content"`
	JobID           string `json:"job_id"`
	StepRunID       string `json:"step_run_id"`
	TemplateID      string `json:"template_id"`
	TemplateVersion int    `json:"template_version"`
}

func (c *MaterialsClient) VerifyReusableGeneratedAsset(ctx context.Context, id, organizationID, scope, ownerUserID string) (*ReusableGeneratedAsset, error) {
	credential := os.Getenv("MEDIA_MATERIALS_INTERNAL_TOKEN")
	if credential == "" || id == "" || organizationID == "" || scope == "" || ownerUserID == "" {
		return nil, errors.New("reusable asset verification is not configured")
	}
	query := url.Values{"organization_id": {organizationID}, "scope": {scope}, "owner_user_id": {ownerUserID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/internal/v1/generated-assets/reusable/"+url.PathEscape(id)+"?"+query.Encode(), nil)
	if err != nil {
		return nil, errors.New("invalid reusable asset verification request")
	}
	req.Header.Set("X-Media-Materials-Internal-Token", credential)
	ctx, span := startDownstreamRequest(ctx, req, "media_materials.VerifyReusableGeneratedAsset")
	defer span.End()
	response, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return nil, errors.New("reusable asset verification unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, ErrReusableAssetNotFound
	}
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("reusable asset verification rejected or unavailable")
	}
	var result ReusableGeneratedAsset
	if json.NewDecoder(response.Body).Decode(&result) != nil || result.ID != id || result.OrganizationID != organizationID || result.Scope != scope || result.OwnerUserID != ownerUserID || result.JobID == "" || result.StepRunID == "" || result.TemplateID == "" || result.TemplateVersion <= 0 {
		return nil, errors.New("invalid reusable asset verification response")
	}
	return &result, nil
}

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
)

var ErrArtifactUploadRejected = errors.New("artifact upload rejected")
var ErrArtifactUploadConflict = errors.New("artifact upload conflict")

type ArtifactUploadRequest struct {
	OrganizationID string `json:"organization_id"`
	Scope          string `json:"scope"`
	UploadKey      string `json:"upload_key"`
	ContentType    string `json:"content_type"`
	SizeBytes      int64  `json:"size_bytes"`
	ChecksumSHA256 string `json:"checksum_sha256"`
	ContentMD5     string `json:"content_md5"`
}

type ArtifactUploadGrant struct {
	ObjectPath      string            `json:"object_path"`
	UploadURL       string            `json:"upload_url"`
	Headers         map[string]string `json:"headers"`
	ExpiresAt       int64             `json:"expires_at"`
	AlreadyUploaded bool              `json:"already_uploaded"`
}

func (c *MaterialsClient) PrepareArtifactUpload(ctx context.Context, input ArtifactUploadRequest) (ArtifactUploadGrant, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return ArtifactUploadGrant{}, errors.New("artifact upload request invalid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/internal/v1/library-objects:prepareUpload", bytes.NewReader(body))
	if err != nil {
		return ArtifactUploadGrant{}, errors.New("artifact upload unavailable")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-Materials-Internal-Token", os.Getenv("MEDIA_MATERIALS_INTERNAL_TOKEN"))
	ctx, span := startDownstreamRequest(ctx, req, "media_materials.PrepareArtifactUpload")
	defer span.End()
	resp, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return ArtifactUploadGrant{}, errors.New("artifact upload unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return ArtifactUploadGrant{}, ErrArtifactUploadConflict
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusRequestEntityTooLarge {
		return ArtifactUploadGrant{}, ErrArtifactUploadRejected
	}
	if resp.StatusCode != http.StatusOK {
		return ArtifactUploadGrant{}, errors.New("artifact upload unavailable")
	}
	var grant ArtifactUploadGrant
	if json.NewDecoder(resp.Body).Decode(&grant) != nil {
		return ArtifactUploadGrant{}, errors.New("artifact upload response invalid")
	}
	return grant, nil
}

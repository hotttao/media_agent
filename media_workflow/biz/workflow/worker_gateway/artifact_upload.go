package worker_gateway

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	capability "media_agent/media_workflow/biz/domain/capability"
	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

type ArtifactMetadata struct {
	CandidateIndex                          int
	ContentType, ChecksumSHA256, ContentMD5 string
	SizeBytes                               int64
}

type PrepareArtifactCommand struct {
	Principal
	DispatchID, WorkerInstanceID string
	Attempt                      int
	ArtifactMetadata
}

type ArtifactUploadPreparer interface {
	PrepareArtifactUpload(context.Context, client.ArtifactUploadRequest) (client.ArtifactUploadGrant, error)
}

func (f *Flow) PrepareArtifactUpload(ctx context.Context, cmd PrepareArtifactCommand) (client.ArtifactUploadGrant, error) {
	dispatch, err := f.authorizeDispatch(ctx, cmd.Principal, cmd.DispatchID, cmd.WorkerInstanceID, cmd.Attempt)
	if err != nil {
		return client.ArtifactUploadGrant{}, err
	}
	return f.prepareArtifactUpload(ctx, dispatch, cmd.ArtifactMetadata)
}

func (r *ManagedRuntime) PrepareArtifactUpload(ctx context.Context, id string, attempt int, meta ArtifactMetadata) (client.ArtifactUploadGrant, error) {
	dispatch, err := r.authorize(ctx, id, attempt)
	if err != nil {
		return client.ArtifactUploadGrant{}, err
	}
	return r.flow.prepareArtifactUpload(ctx, dispatch, meta)
}

func (f *Flow) prepareArtifactUpload(ctx context.Context, d *domain.WorkerDispatch, meta ArtifactMetadata) (client.ArtifactUploadGrant, error) {
	sha, shaErr := hex.DecodeString(meta.ChecksumSHA256)
	md5, md5Err := base64.StdEncoding.DecodeString(meta.ContentMD5)
	allowed := map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true, "video/mp4": true, "video/webm": true}
	if meta.CandidateIndex < 0 || meta.CandidateIndex >= 8 || !allowed[meta.ContentType] || meta.SizeBytes <= 0 || meta.SizeBytes > 256<<20 ||
		strings.HasPrefix(meta.ContentType, "image/") && meta.SizeBytes > 16<<20 || shaErr != nil || len(sha) != 32 || strings.ToLower(meta.ChecksumSHA256) != meta.ChecksumSHA256 ||
		md5Err != nil || len(md5) != 16 || base64.StdEncoding.EncodeToString(md5) != meta.ContentMD5 {
		return client.ArtifactUploadGrant{}, ErrInvalidRequest
	}
	if f.ArtifactUploads == nil || f.Capabilities == nil {
		return client.ArtifactUploadGrant{}, errors.New("artifact upload unavailable")
	}
	c, err := f.Capabilities.Get(ctx, d.CapabilityKey, d.CapabilityVersion)
	if err != nil {
		return client.ArtifactUploadGrant{}, errors.New("artifact capability unavailable")
	}
	if c == nil || (c.OutputMediaType != capability.MediaImage && c.OutputMediaType != capability.MediaVideo) || !strings.HasPrefix(meta.ContentType, strings.ToLower(string(c.OutputMediaType))+"/") {
		return client.ArtifactUploadGrant{}, ErrInvalidOutput
	}
	if (d.AuthSource == "api_token" && c.ExecutionTarget != capability.ExecutionTargetPersonalWorker) || (d.AuthSource == "internal_runtime" && c.ExecutionTarget != capability.ExecutionTargetManagedWorker) {
		return client.ArtifactUploadGrant{}, ErrForbidden
	}
	j, err := f.Jobs.Get(ctx, d.JobID)
	if err != nil {
		return client.ArtifactUploadGrant{}, errors.New("artifact job unavailable")
	}
	if j.OrganizationID != d.OrganizationID || j.SubmittedBy != d.UserID || j.ConductorWorkflowID != d.ConductorWorkflowID || j.Scope == "" {
		return client.ArtifactUploadGrant{}, ErrForbidden
	}
	key := sha256.Sum256([]byte(strings.Join([]string{d.ConductorTaskID, strconv.Itoa(d.Attempt), strconv.Itoa(meta.CandidateIndex)}, "\x00")))
	grant, err := f.ArtifactUploads.PrepareArtifactUpload(ctx, client.ArtifactUploadRequest{OrganizationID: j.OrganizationID, Scope: j.Scope, UploadKey: hex.EncodeToString(key[:]), ContentType: meta.ContentType, SizeBytes: meta.SizeBytes, ChecksumSHA256: meta.ChecksumSHA256, ContentMD5: meta.ContentMD5})
	if errors.Is(err, client.ErrArtifactUploadRejected) {
		return client.ArtifactUploadGrant{}, ErrInvalidRequest
	}
	if errors.Is(err, client.ErrArtifactUploadConflict) {
		return client.ArtifactUploadGrant{}, domain.ErrDispatchConflict
	}
	if err != nil {
		return client.ArtifactUploadGrant{}, errors.New("artifact upload unavailable")
	}
	prefix := fmt.Sprintf("materials/%s/%s/worker_generated/%x/", j.OrganizationID, j.Scope, key[:])
	if !strings.HasPrefix(grant.ObjectPath, prefix) || path.Clean(grant.ObjectPath) != grant.ObjectPath || strings.Contains(grant.ObjectPath, "\\") ||
		(!grant.AlreadyUploaded && (grant.UploadURL == "" || grant.ExpiresAt <= f.now().Unix())) {
		return client.ArtifactUploadGrant{}, errors.New("artifact upload response invalid")
	}
	if !grant.AlreadyUploaded {
		u, err := url.Parse(grant.UploadURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
			return client.ArtifactUploadGrant{}, errors.New("artifact upload response invalid")
		}
		if f.StorageOrigin != "" {
			origin, err := url.Parse(f.StorageOrigin)
			if err != nil || u.Scheme != origin.Scheme || u.Host != origin.Host || !strings.HasPrefix(u.Path, strings.TrimRight(origin.Path, "/")+"/") {
				return client.ArtifactUploadGrant{}, errors.New("artifact upload response invalid")
			}
		}
		for key := range grant.Headers {
			lower := strings.ToLower(key)
			if lower != "host" && lower != "if-none-match" && lower != "content-md5" && lower != "content-type" && lower != "content-length" && !strings.HasPrefix(lower, "x-amz-") {
				return client.ArtifactUploadGrant{}, errors.New("artifact upload response invalid")
			}
		}
	}
	return grant, nil
}

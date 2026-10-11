package reuse_task

import (
	"context"
	capability "media_agent/media_workflow/biz/domain/capability"
	"mime"
	"net/url"
	"path"
	"strings"

	"media_agent/media_workflow/biz/shared/client"
)

type StoredObjectLookup interface {
	ResolveInternalObject(context.Context, string, string) (client.ResolvedStorageObject, error)
}

type StorageDigests struct {
	Objects       StoredObjectLookup
	StorageOrigin string
	StorageBucket string
}

func (r StorageDigests) Checksum(ctx context.Context, organizationID, reference string, expected capability.MediaType) (string, error) {
	if r.Objects == nil {
		return "", ErrMediaResolverUnavailable
	}
	objectPath, err := r.objectPath(organizationID, reference)
	if err != nil {
		return "", err
	}
	object, err := r.Objects.ResolveInternalObject(ctx, organizationID, objectPath)
	if err != nil {
		return "", err
	}
	if object.SizeBytes <= 0 {
		return "", ErrInvalidFingerprintInput
	}
	if expected != "" {
		contentType, _, err := mime.ParseMediaType(object.ContentType)
		if err != nil || !expected.Valid() || !strings.HasPrefix(contentType, strings.ToLower(string(expected))+"/") {
			return "", ErrInvalidFingerprintInput
		}
	}
	return object.ChecksumSHA256, nil
}

func (r StorageDigests) objectPath(organizationID, reference string) (string, error) {
	if organizationID == "" || strings.ContainsAny(organizationID, "/\\") {
		return "", ErrInvalidFingerprintInput
	}
	objectPath := strings.TrimPrefix(reference, "object_path:")
	if strings.Contains(reference, "://") {
		origin, err := url.Parse(r.StorageOrigin)
		if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" || (origin.Scheme != "http" && origin.Scheme != "https") {
			return "", ErrInvalidFingerprintInput
		}
		parsed, err := url.Parse(reference)
		bucket := r.StorageBucket
		if bucket == "" {
			bucket = "media"
		}
		if strings.ContainsAny(bucket, "/\\?#") || err != nil || parsed.Host != origin.Host || parsed.Scheme != origin.Scheme || parsed.User != nil || parsed.Fragment != "" || !strings.HasPrefix(parsed.Path, "/"+bucket+"/") {
			return "", ErrInvalidFingerprintInput
		}
		objectPath = strings.TrimPrefix(parsed.Path, "/"+bucket+"/")
	}
	if !strings.HasPrefix(objectPath, "materials/"+organizationID+"/") || path.Clean(objectPath) != objectPath || strings.ContainsAny(objectPath, "\\?#") {
		return "", ErrInvalidFingerprintInput
	}
	return objectPath, nil
}

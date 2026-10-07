package worker_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"strings"

	capability "media_agent/media_workflow/biz/domain/capability"
	job "media_agent/media_workflow/biz/domain/job"
)

// Validate before the first native completion, not on acknowledged replay.
// This performs no Asset registration and never persists temporary URLs.
func (f *Flow) validateOutputMedia(ctx context.Context, d *job.WorkerDispatch, j *job.Job, output map[string]any) error {
	if f.Capabilities == nil {
		return errors.New("artifact capability unavailable")
	}
	c, err := f.Capabilities.Get(ctx, d.CapabilityKey, d.CapabilityVersion)
	if err != nil || c == nil {
		return errors.New("artifact capability unavailable")
	}
	if c.OutputMediaType == "" || c.OutputMediaType == capability.MediaText {
		return nil
	}
	if c.ValidateContract() != nil {
		return ErrInvalidOutput
	}
	if f.ImportedAssets == nil {
		return errors.New("artifact storage unavailable")
	}
	var candidates []map[string]any
	switch raw := output["candidates"].(type) {
	case []map[string]any:
		candidates = raw
	case []any:
		for _, item := range raw {
			candidate, ok := item.(map[string]any)
			if !ok {
				return ErrInvalidOutput
			}
			candidates = append(candidates, candidate)
		}
	default:
		return ErrInvalidOutput
	}
	if len(candidates) == 0 || len(candidates) > 8 {
		return ErrInvalidOutput
	}
	for index, candidate := range candidates {
		if !candidatePosition(candidate["index"], index) {
			return ErrInvalidOutput
		}
		for _, field := range []string{"object_path", "url", "content"} {
			if value, present := candidate[field]; present && value != nil {
				if _, ok := value.(string); !ok {
					return ErrInvalidOutput
				}
			}
		}
		if value, present := candidate["is_primary"]; present && value != nil {
			if _, ok := value.(bool); !ok {
				return ErrInvalidOutput
			}
		}
		objectPath, _ := candidate["object_path"].(string)
		rawURL, _ := candidate["url"].(string)
		content, _ := candidate["content"].(string)
		if content != "" || (objectPath != "" && rawURL != "") {
			return ErrInvalidOutput
		}
		if objectPath == "" {
			// Legacy fixture URLs are accepted only from the configured store.
			origin, originErr := url.Parse(f.StorageOrigin)
			u, urlErr := url.Parse(rawURL)
			if originErr != nil || urlErr != nil || origin.Host == "" || u.Host != origin.Host || u.Scheme != origin.Scheme || u.User != nil || u.Fragment != "" || !strings.HasPrefix(u.Path, "/media/") {
				return ErrInvalidOutput
			}
			objectPath = strings.TrimPrefix(u.Path, "/media/")
		}
		prefix := "materials/" + j.OrganizationID + "/" + j.Scope + "/"
		if j.Scope == "" || !strings.HasPrefix(objectPath, prefix) || path.Clean(objectPath) != objectPath || strings.Contains(objectPath, "\\") {
			return ErrInvalidOutput
		}
		object, err := f.ImportedAssets.ResolveInternalObject(ctx, j.OrganizationID, objectPath)
		if err != nil {
			return errors.New("artifact storage unavailable")
		}
		allowed := map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true, "video/mp4": true, "video/webm": true}
		if !allowed[object.ContentType] || !strings.HasPrefix(object.ContentType, strings.ToLower(string(c.OutputMediaType))+"/") || object.SizeBytes <= 0 || object.SizeBytes > 256<<20 || c.OutputMediaType == capability.MediaImage && object.SizeBytes > 16<<20 {
			return ErrInvalidOutput
		}
	}
	return nil
}

func candidatePosition(value any, position int) bool {
	switch n := value.(type) {
	case int:
		return n == position
	case int32:
		return int64(n) == int64(position)
	case int64:
		return n == int64(position)
	case float64:
		return n == float64(position)
	case json.Number:
		actual, err := n.Int64()
		return err == nil && actual == int64(position)
	default:
		return false
	}
}

package reuse_task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	capability "media_agent/media_workflow/biz/domain/capability"
)

var (
	ErrDigestUnavailable        = errors.New("media content digest unavailable")
	ErrMediaResolverUnavailable = errors.New("media digest resolver unavailable")
	ErrInvalidFingerprintInput  = errors.New("invalid generation fingerprint input")
)

// Identity pins the generating contract, not the Job, Step key or delivery.
type Identity struct {
	TemplateID        string             `json:"template_id"`
	TemplateVersion   int                `json:"template_version"`
	CapabilityKey     string             `json:"capability_key"`
	CapabilityVersion int                `json:"capability_version"`
	CandidateCount    int                `json:"candidate_count"`
	OutputAssetKind   string             `json:"output_asset_kind"`
	Inputs            []capability.Input `json:"inputs"`
}

type MediaDigestResolver interface {
	Checksum(context.Context, string, string, capability.MediaType) (string, error)
}

// BuildFingerprint replaces declared media references with their content SHA,
// retaining media order. It never fetches arbitrary URLs or hashes URL text.
func BuildFingerprint(ctx context.Context, organizationID string, identity Identity, input map[string]any, resolver MediaDigestResolver) (string, error) {
	if organizationID == "" || identity.TemplateID == "" || identity.TemplateVersion <= 0 || identity.CapabilityKey == "" || identity.CapabilityVersion <= 0 || identity.OutputAssetKind == "" || identity.CandidateCount <= 0 {
		return "", ErrInvalidFingerprintInput
	}
	names := make(map[string]bool, len(identity.Inputs))
	values := make(map[string]any, len(identity.Inputs))
	for _, field := range identity.Inputs {
		if field.Name == "" || names[field.Name] || !field.ValidMediaConstraint() {
			return "", ErrInvalidFingerprintInput
		}
		names[field.Name] = true
		switch field.Type {
		case capability.InputString, capability.InputResourceRef, capability.InputResourceRefList:
		default:
			return "", ErrInvalidFingerprintInput
		}
		value, exists := input[field.Name]
		if !exists {
			if field.Required {
				return "", ErrInvalidFingerprintInput
			}
			continue
		}
		if value == nil {
			if field.Required {
				return "", ErrInvalidFingerprintInput
			}
			values[field.Name] = nil
			continue
		}
		switch field.Type {
		case capability.InputString:
			text, ok := value.(string)
			if !ok || (field.Required && strings.TrimSpace(text) == "") {
				return "", ErrInvalidFingerprintInput
			}
			values[field.Name] = text
		case capability.InputResourceRef:
			ref, ok := value.(string)
			if !ok || ref == "" {
				return "", ErrInvalidFingerprintInput
			}
			digest, err := mediaChecksum(ctx, organizationID, ref, field.MediaType, resolver)
			if err != nil {
				return "", err
			}
			values[field.Name] = map[string]string{"sha256": digest}
		case capability.InputResourceRefList:
			refs, err := resourceRefs(value)
			if err != nil || !field.AcceptsResourceCount(len(refs)) {
				return "", ErrInvalidFingerprintInput
			}
			digests := make([]map[string]string, 0, len(refs))
			for _, ref := range refs {
				digest, err := mediaChecksum(ctx, organizationID, ref, field.MediaType, resolver)
				if err != nil {
					return "", err
				}
				digests = append(digests, map[string]string{"sha256": digest})
			}
			values[field.Name] = digests
		default:
			return "", ErrInvalidFingerprintInput
		}
	}
	for name := range input {
		if names[name] {
			continue
		}
		switch name {
		case "job_id", "step_key", "capability_key", "capability_version":
		default:
			return "", ErrInvalidFingerprintInput
		}
	}
	// encoding/json sorts string map keys. No map traversal order or ephemeral
	// storage address is allowed to affect this versioned canonical envelope.
	payload, err := json.Marshal(struct {
		Schema   int            `json:"schema"`
		Identity Identity       `json:"identity"`
		Values   map[string]any `json:"values"`
	}{1, identity, values})
	if err != nil {
		return "", ErrInvalidFingerprintInput
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func mediaChecksum(ctx context.Context, organizationID, ref string, mediaType capability.MediaType, resolver MediaDigestResolver) (string, error) {
	if ref == "" {
		return "", ErrInvalidFingerprintInput
	}
	if resolver == nil {
		return "", ErrMediaResolverUnavailable
	}
	digest, err := resolver.Checksum(ctx, organizationID, ref, mediaType)
	if err != nil {
		return "", err
	}
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != sha256.Size {
		return "", ErrDigestUnavailable
	}
	return strings.ToLower(digest), nil
}

func resourceRefs(value any) ([]string, error) {
	if refs, ok := value.([]string); ok {
		return refs, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, ErrInvalidFingerprintInput
	}
	refs := make([]string, 0, len(values))
	for _, item := range values {
		ref, ok := item.(string)
		if !ok {
			return nil, ErrInvalidFingerprintInput
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

package capability

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type GenerationMethod string
type GenerationType string

const (
	TextToImage      GenerationMethod = "t2i"
	ImageToImage     GenerationMethod = "i2i"
	TextToVideo      GenerationMethod = "t2v"
	ImageToVideo     GenerationMethod = "i2v"
	FirstFramePrompt GenerationType   = "first_frame_prompt"
	MultiImagePrompt GenerationType   = "multi_image_prompt"
	TextPrompt       GenerationType   = "text_prompt"
)

var ErrInvalidContract = errors.New("invalid provider capability contract")
var platformName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func (m GenerationMethod) OutputMedia() MediaType {
	switch m {
	case TextToImage, ImageToImage:
		return MediaImage
	case TextToVideo, ImageToVideo:
		return MediaVideo
	}
	return ""
}

func ContractKey(platform string, method GenerationMethod, kind GenerationType, target ExecutionTarget) (string, error) {
	if !platformName.MatchString(platform) || platform == "client" || !target.Valid() {
		return "", ErrInvalidContract
	}
	switch kind {
	case FirstFramePrompt, MultiImagePrompt:
		if method != ImageToImage && method != ImageToVideo {
			return "", ErrInvalidContract
		}
	case TextPrompt:
		if method != TextToImage && method != TextToVideo {
			return "", ErrInvalidContract
		}
	default:
		return "", ErrInvalidContract
	}
	key := platform + "." + string(method) + "." + string(kind)
	if target == ExecutionTargetManagedWorker {
		key += ".managed"
	}
	return key, nil
}

func (c Capability) Ref() string { return fmt.Sprintf("%s@%d", c.Key, c.Version) }

// ValidateContract does not reinterpret legacy contracts. New contracts have
// canonical identity, typed output and a fixed project Worker signature.
func (c Capability) ValidateContract() error {
	if c.OutputMediaType == "" {
		if c.GenerationMethod != "" || c.GenerationType != "" || c.OutputAssetKind == "" {
			return ErrInvalidContract
		}
		return nil
	}
	key, err := ContractKey(c.Provider, c.GenerationMethod, c.GenerationType, c.ExecutionTarget)
	if err != nil || key != c.Key || c.Version <= 0 || c.Version > 2147483647 || c.OutputAssetKind != "" || c.OutputMediaType != c.GenerationMethod.OutputMedia() || c.ConductorTaskDef == "" {
		return ErrInvalidContract
	}
	if (c.ExecutionTarget == ExecutionTargetPersonalWorker && c.ManagedPoolKey != "") || (c.ExecutionTarget == ExecutionTargetManagedWorker && strings.TrimSpace(c.ManagedPoolKey) == "") {
		return ErrInvalidContract
	}
	if (c.OutputMediaType == MediaImage && c.Mode != "image_generation") || (c.OutputMediaType == MediaVideo && c.Mode != "video_generation") {
		return ErrInvalidContract
	}
	expected := 2
	if c.GenerationType == TextPrompt {
		expected = 1
	}
	if len(c.Inputs) != expected {
		return ErrInvalidContract
	}
	seen := map[string]bool{}
	for _, input := range c.Inputs {
		if seen[input.Name] || !input.Required || !input.ValidMediaConstraint() {
			return ErrInvalidContract
		}
		seen[input.Name] = true
		switch input.Name {
		case "prompt":
			if input.Type != InputString || input.MediaType != "" {
				return ErrInvalidContract
			}
		case "images":
			if c.GenerationType == TextPrompt || input.Type != InputResourceRefList || input.MediaType != MediaImage || input.MinItems != 1 {
				return ErrInvalidContract
			}
			if c.GenerationType == FirstFramePrompt && input.MaxItems != 1 {
				return ErrInvalidContract
			}
		default:
			return ErrInvalidContract
		}
	}
	if !seen["prompt"] || (expected == 2 && !seen["images"]) {
		return ErrInvalidContract
	}
	return nil
}

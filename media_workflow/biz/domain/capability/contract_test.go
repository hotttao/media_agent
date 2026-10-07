package capability

import (
	"errors"
	"testing"
)

func providerContract(platform string, method GenerationMethod, kind GenerationType) Capability {
	key, _ := ContractKey(platform, method, kind, ExecutionTargetPersonalWorker)
	mode := "image_generation"
	if method.OutputMedia() == MediaVideo {
		mode = "video_generation"
	}
	images := Input{Name: "images", Type: InputResourceRefList, Required: true, MediaType: MediaImage, MinItems: 1}
	if kind == FirstFramePrompt {
		images.MaxItems = 1
	}
	return Capability{Key: key, Version: 1, Provider: platform, Mode: mode, GenerationMethod: method, GenerationType: kind, OutputMediaType: method.OutputMedia(), ConductorTaskDef: "provider_v1", ExecutionTarget: ExecutionTargetPersonalWorker, Inputs: []Input{{Name: "prompt", Type: InputString, Required: true}, images}}
}

func TestOriginalFourProviderContractsHaveDistinctIdentity(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range []Capability{providerContract("jimeng", ImageToImage, FirstFramePrompt), providerContract("jimeng", ImageToImage, MultiImagePrompt), providerContract("doubao", ImageToVideo, FirstFramePrompt), providerContract("doubao", ImageToVideo, MultiImagePrompt)} {
		if c.ValidateContract() != nil || seen[c.Ref()] {
			t.Fatal("original task identity lost or contract invalid")
		}
		seen[c.Ref()] = true
		if c.OutputAssetKind != "" {
			t.Fatal("provider identity coupled to business asset kind")
		}
	}
	if len(seen) != 4 {
		t.Fatal("not all original modes represented")
	}
}

func TestProviderContractRejectsInconsistentIdentityAndSignature(t *testing.T) {
	for _, mutation := range []string{"key", "version", "platform", "method", "type", "output", "business_kind", "mode", "missing_prompt", "duplicate_prompt", "resources", "required", "media", "min", "max", "negative", "pool"} {
		t.Run(mutation, func(t *testing.T) {
			c := providerContract("jimeng", ImageToImage, FirstFramePrompt)
			switch mutation {
			case "key":
				c.Key = "media.image.model"
			case "version":
				c.Version = 0
			case "platform":
				c.Provider = "client"
			case "method":
				c.GenerationMethod = TextToImage
			case "type":
				c.GenerationType = "model_image"
			case "output":
				c.OutputMediaType = MediaVideo
			case "business_kind":
				c.OutputAssetKind = "model_image"
			case "mode":
				c.Mode = "video_generation"
			case "missing_prompt":
				c.Inputs = c.Inputs[1:]
			case "duplicate_prompt":
				c.Inputs[1] = c.Inputs[0]
			case "resources":
				c.Inputs[1].Name = "resources"
			case "required":
				c.Inputs[1].Required = false
			case "media":
				c.Inputs[1].MediaType = MediaVideo
			case "min":
				c.Inputs[1].MinItems = 0
			case "max":
				c.Inputs[1].MaxItems = 0
			case "negative":
				c.Inputs[1].MinItems = -1
			case "pool":
				c.ManagedPoolKey = "managed"
			}
			if !errors.Is(c.ValidateContract(), ErrInvalidContract) {
				t.Fatal("inconsistent provider contract accepted")
			}
		})
	}
}

func TestLegacyContractDoesNotGuessProviderMode(t *testing.T) {
	c := Capability{Key: "media.image.model", Version: 1, Provider: "client", Mode: "image_generation", OutputAssetKind: "model_image"}
	if c.ValidateContract() != nil || c.GenerationMethod != "" || c.GenerationType != "" || c.OutputMediaType != "" {
		t.Fatal("legacy capability reinterpreted")
	}
	c.GenerationMethod = ImageToImage
	if !errors.Is(c.ValidateContract(), ErrInvalidContract) {
		t.Fatal("partial legacy conversion accepted")
	}
}

func TestManagedContractUsesSeparateKeyAndRequiresPool(t *testing.T) {
	c := providerContract("jimeng", ImageToImage, MultiImagePrompt)
	c.ExecutionTarget = ExecutionTargetManagedWorker
	c.Key, _ = ContractKey(c.Provider, c.GenerationMethod, c.GenerationType, c.ExecutionTarget)
	if c.Key != "jimeng.i2i.multi_image_prompt.managed" || c.ValidateContract() == nil {
		t.Fatal("managed identity or pool boundary lost")
	}
	c.ManagedPoolKey = "trusted-pool"
	if c.ValidateContract() != nil {
		t.Fatal("valid managed contract rejected")
	}
}

func TestFirstFrameAndMultiImageContractsKeepDistinctCardinality(t *testing.T) {
	first := providerContract("jimeng", ImageToImage, FirstFramePrompt).Inputs[1]
	multi := providerContract("jimeng", ImageToImage, MultiImagePrompt).Inputs[1]
	if first.AcceptsResourceCount(0) || !first.AcceptsResourceCount(1) || first.AcceptsResourceCount(2) {
		t.Fatal("first-frame cardinality lost")
	}
	if multi.AcceptsResourceCount(0) || !multi.AcceptsResourceCount(1) || !multi.AcceptsResourceCount(3) {
		t.Fatal("multi-image cardinality lost")
	}
	if (Input{Type: InputString}).AcceptsResourceCount(1) || (Input{Type: InputResourceRefList, MinItems: 2, MaxItems: 1}).AcceptsResourceCount(1) {
		t.Fatal("invalid count constraint accepted")
	}
}

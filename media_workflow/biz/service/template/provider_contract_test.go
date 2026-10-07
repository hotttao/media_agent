package template

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	capability "media_agent/media_workflow/biz/domain/capability"
	"media_agent/media_workflow/biz/shared"
)

type contractRegistry struct {
	capability.Repo
	value *capability.Capability
}

func (r contractRegistry) Get(context.Context, string, int) (*capability.Capability, error) {
	return r.value, nil
}

type contractKinds struct {
	values map[string]*shared.AssetKindInfo
	err    error
}

func (r contractKinds) BatchGet(_ context.Context, names []string) (map[string]*shared.AssetKindInfo, []string, error) {
	var missing []string
	for _, name := range names {
		if r.values[name] == nil {
			missing = append(missing, name)
		}
	}
	return r.values, missing, r.err
}

func TestTemplateProviderCapabilityUsesRegisteredMediaNotBusinessKind(t *testing.T) {
	registry := contractRegistry{value: &capability.Capability{Key: "jimeng.i2i.multi_image_prompt", Version: 1, OutputMediaType: capability.MediaImage}}
	kinds := contractKinds{values: map[string]*shared.AssetKindInfo{
		"model_image": {Name: "model_image", ResourceType: "ASSET", MediaType: "IMAGE", Status: "SYSTEM"},
		"style_image": {Name: "style_image", ResourceType: "ASSET", MediaType: "IMAGE", Status: "ENABLED"},
		"video":       {Name: "video", ResourceType: "ASSET", MediaType: "VIDEO", Status: "SYSTEM"},
		"material":    {Name: "material", ResourceType: "MATERIAL", MediaType: "IMAGE", Status: "SYSTEM"},
		"disabled":    {Name: "disabled", ResourceType: "ASSET", MediaType: "IMAGE", Status: "DISABLED"},
	}}
	svc := NewService(nil, nil, nil, kinds, registry)
	for _, kind := range []string{"model_image", "style_image"} {
		require.NoError(t, svc.validateCapability(context.Background(), registry.value.Key, 1, kind))
	}
	for _, kind := range []string{"video", "material", "disabled", "missing", ""} {
		require.Error(t, svc.validateCapability(context.Background(), registry.value.Key, 1, kind))
	}
	svc.kindLookup = contractKinds{err: errors.New("metadata offline")}
	require.Error(t, svc.validateCapability(context.Background(), registry.value.Key, 1, "model_image"))
	svc.kindLookup = nil
	require.Error(t, svc.validateCapability(context.Background(), registry.value.Key, 1, "model_image"))
}

func TestTemplateLegacyCapabilityDoesNotGuessTechnicalMedia(t *testing.T) {
	registry := contractRegistry{value: &capability.Capability{Key: "media.image.model", Version: 1, OutputAssetKind: "model_image"}}
	svc := NewService(nil, nil, nil, nil, registry)
	require.NoError(t, svc.validateCapability(context.Background(), registry.value.Key, 1, "model_image"))
	require.Error(t, svc.validateCapability(context.Background(), registry.value.Key, 1, "style_image"))
}

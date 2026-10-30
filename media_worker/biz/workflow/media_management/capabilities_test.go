package media_management

import (
	"context"
	contract "media_agent/media_worker/biz/shared/media_management"
	"strings"
	"testing"
)

type capabilityCatalog struct{ calls []string }

func (c *capabilityCatalog) Execute(_ context.Context, operation string, p contract.Params) (map[string]any, error) {
	c.calls = append(c.calls, p.CapabilityKey)
	provider := "gpt"
	if strings.HasPrefix(p.CapabilityKey, "doubao.") {
		provider = "doubao"
	}
	return map[string]any{"key": p.CapabilityKey, "provider": provider, "status": "published"}, nil
}
func TestCapabilityDiscoveryIncludesRealMultiImageVideo(t *testing.T) {
	api := &capabilityCatalog{}
	result, err := (Operations{API: api}).capabilities(context.Background(), contract.Params{Provider: "doubao", Page: 1, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	items := result["items"].([]any)
	if len(api.calls) != 4 || len(items) != 2 || items[1].(map[string]any)["key"] != "doubao.i2v.multi_image_prompt" {
		t.Fatal("multi-image video missing", result, api.calls)
	}
}

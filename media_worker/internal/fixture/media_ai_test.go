package fixture

import (
	"strings"
	"testing"

	"media_agent/media_worker/internal/gateway"
)

func TestFixtureUsesConfiguredAssetServiceAndFrozenProvenance(t *testing.T) {
	executor, err := NewExecutor("https://assets.example.test/media/")
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range Capabilities() {
		output, err := executor.Execute(gateway.Task{CapabilityKey: capability.Key, CapabilityVersion: capability.Version})
		if err != nil {
			t.Fatal(err)
		}
		candidate := output["candidates"].([]map[string]any)[0]
		if !strings.HasPrefix(candidate["url"].(string), "https://assets.example.test/media/uploads/teams/") {
			t.Fatal("configured resource service was not used")
		}
		provenance := output["provenance"].(map[string]any)
		if provenance["mock"] != true || provenance["source"] != source {
			t.Fatal("mock provenance lost")
		}
	}
	for _, task := range []gateway.Task{
		{CapabilityKey: "media_render_template_v1", CapabilityVersion: 1},
		{CapabilityKey: "media.image.model", CapabilityVersion: 2},
	} {
		if _, err := executor.Execute(task); err == nil {
			t.Fatal("unsupported task must not return a frozen result")
		}
	}
}

func TestFixtureRejectsInvalidOrSensitiveAssetBase(t *testing.T) {
	for _, base := range []string{"", "/assets", "file:///tmp/assets", "https://user:secret@example.test", "https://example.test?token=secret", "https://example.test#fragment"} {
		if _, err := NewExecutor(base); err == nil {
			t.Fatal("unsafe fixture base accepted")
		}
	}
}

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

func TestFixtureCanReturnPreuploadedVideoResult(t *testing.T) {
	executor, err := NewExecutor("https://legacy.example")
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"file:///tmp/video.mp4", "https://user:pass@example.test/video.mp4", "https://example.test/video.mp4#fragment"} {
		if err := executor.SetVideoResultURL(invalid); err == nil {
			t.Fatalf("unsafe fixture video URL accepted: %q", invalid)
		}
	}
	videoURL := "https://storage.example/media/materials/org/ai_clothing/generated_asset_import/video.mp4?signature=short-lived"
	if err := executor.SetVideoResultURL(videoURL); err != nil {
		t.Fatal(err)
	}
	output, err := executor.Execute(gateway.Task{CapabilityKey: "media.video.generate", CapabilityVersion: 1})
	if err != nil || output["candidates"].([]map[string]any)[0]["url"] != videoURL {
		t.Fatalf("fixture did not return controlled video: output=%+v err=%v", output, err)
	}
}

func TestFixtureAllCapabilitiesUseControlledResults(t *testing.T) {
	executor, err := NewExecutor("https://legacy.example")
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range Capabilities() {
		resultURL := "https://storage.example/materials/org/scope/" + capability.Key + "?signature=test-only"
		if err := executor.SetResultURL(capability.Key, resultURL); err != nil {
			t.Fatal(err)
		}
		for _, invalid := range []string{"/relative", "file:///tmp/result", "https://user:secret@example.test/result", "https://example.test/result#fragment"} {
			if err := executor.SetResultURL(capability.Key, invalid); err == nil || strings.Contains(err.Error(), invalid) {
				t.Fatal("unsafe override accepted or exposed in error")
			}
		}
		output, err := executor.Execute(gateway.Task{CapabilityKey: capability.Key, CapabilityVersion: 1})
		if err != nil {
			t.Fatal(err)
		}
		if output["candidates"].([]map[string]any)[0]["url"] != resultURL {
			t.Fatal("controlled result was not preserved after invalid configuration")
		}
		provenance := output["provenance"].(map[string]any)
		if provenance["mock"] != true || provenance["source"] != source {
			t.Fatal("controlled result lost mock provenance")
		}
		if err := executor.SetResultURL(capability.Key, ""); err != nil {
			t.Fatal(err)
		}
		fallback, err := executor.Execute(gateway.Task{CapabilityKey: capability.Key, CapabilityVersion: 1})
		if err != nil || !strings.HasPrefix(fallback["candidates"].([]map[string]any)[0]["url"].(string), "https://legacy.example/") {
			t.Fatal("cleared override did not restore the existing fixture")
		}
	}
	for _, key := range []string{"media_render_template_v1", "media_finalize_step_v1", "unknown"} {
		if err := executor.SetResultURL(key, "https://storage.example/result"); err == nil {
			t.Fatal("internal or unknown task accepted as a fixture Capability")
		}
		if _, err := executor.Execute(gateway.Task{CapabilityKey: key, CapabilityVersion: 1}); err == nil {
			t.Fatal("internal or unknown task was executed by the client Worker")
		}
	}
	if _, err := executor.Execute(gateway.Task{CapabilityKey: "media.image.model", CapabilityVersion: 2}); err == nil {
		t.Fatal("unknown Capability Version accepted")
	}
}

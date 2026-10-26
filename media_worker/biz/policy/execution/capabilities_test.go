package execution

import (
	"testing"

	"media_agent/media_worker/biz/shared/worker_runtime"
)

func TestSelectCapabilities(t *testing.T) {
	available := []worker_runtime.Capability{
		{Key: "media.image.model", Version: 1, Concurrency: 1},
		{Key: "media.video.generate", Version: 1, Concurrency: 2},
	}
	selected, err := SelectCapabilities(available, "media.video.generate@1,media.video.generate@1")
	if err != nil {
		t.Fatalf("SelectCapabilities() error = %v", err)
	}
	if len(selected) != 1 || selected[0].Key != "media.video.generate" || selected[0].Concurrency != 2 {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestSelectCapabilitiesRejectsUnknownVersion(t *testing.T) {
	_, err := SelectCapabilities([]worker_runtime.Capability{{Key: "media.image.model", Version: 1}}, "media.image.model@2")
	if err == nil {
		t.Fatal("SelectCapabilities() should reject an unsupported capability version")
	}
}

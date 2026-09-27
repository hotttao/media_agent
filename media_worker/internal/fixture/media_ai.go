package fixture

import (
	"fmt"
	"strings"

	"media_agent/media_worker/internal/gateway"
)

const source = "media_ai_fixture:video_task:a6b51312-868e-41dc-a169-698389559703"

var capabilities = []gateway.Capability{
	{Key: "media.image.model", Version: 1, Concurrency: 1},
	{Key: "media.image.style", Version: 1, Concurrency: 1},
	{Key: "media.image.first_frame", Version: 1, Concurrency: 1},
	{Key: "media.video.generate", Version: 1, Concurrency: 1},
}

func Capabilities() []gateway.Capability { return append([]gateway.Capability(nil), capabilities...) }

func Execute(task gateway.Task) (map[string]any, error) {
	url, content := fixtureResult(task)
	if url == "" && content == "" {
		return nil, fmt.Errorf("media_ai fixture does not support capability %q", task.CapabilityKey)
	}
	return map[string]any{
		"candidates": []map[string]any{{"index": 0, "url": url, "content": content, "is_primary": true}},
		"provenance": map[string]any{"profile": "media_ai_fixture", "source": source, "mock": true},
	}, nil
}

func fixtureResult(task gateway.Task) (string, string) {
	const base = "http://192.168.2.89:3000"
	switch task.CapabilityKey {
	case "media.image.model":
		return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/model-images/result-01_afbaea05-ec31-4a49-824c-ba0fd1889655.png", ""
	case "media.image.style":
		return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/style-images/result-01_37108e6c-472e-47e8-8ca6-321fa1c1bafc.png", ""
	case "media.image.first_frame":
		return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/first-frame-images/result-01_659b03f1-91e8-4dc5-b051-e10becc6e810.png", ""
	case "media.video.generate":
		stepKey, _ := task.Input["step_key"].(string)
		if strings.Contains(stepKey, "video_b") {
			return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/videos/video-01_1a74ce8a-9e5f-4678-a62b-427dfcb7758e.mp4", ""
		}
		return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/videos/video-01_805cbdb8-3ee9-45a9-bbe6-cc2ee5204062.mp4", ""
	default:
		return "", ""
	}
}

package fixture

import (
	"fmt"
	"strings"

	"media_agent/media_worker/internal/conductor"
)

const source = "media_ai_fixture:video_task:a6b51312-868e-41dc-a169-698389559703"

var capabilityTasks = []string{
	"media_image_model_v1",
	"media_image_style_v1",
	"media_image_first_frame_v1",
	"media_video_generate_v1",
	"media_video_clip_v1",
	"media_content_ai_fill_v1",
	"media_publish_feishu_v1",
}

func Tasks() []string { return append([]string(nil), capabilityTasks...) }

func Execute(task conductor.Task) (map[string]any, error) {
	url, content := fixtureResult(task)
	if url == "" && content == "" {
		return nil, fmt.Errorf("media_ai fixture does not support task %q", task.TaskType)
	}
	return map[string]any{
		"candidates": []map[string]any{{"index": 0, "url": url, "content": content, "is_primary": true}},
		"provenance": map[string]any{"profile": "media_ai_fixture", "source": source, "mock": true},
	}, nil
}

func fixtureResult(task conductor.Task) (string, string) {
	const base = "http://192.168.2.89:3000"
	switch task.TaskType {
	case "media_image_model_v1":
		return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/model-images/result-01_afbaea05-ec31-4a49-824c-ba0fd1889655.png", ""
	case "media_image_style_v1":
		return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/style-images/result-01_37108e6c-472e-47e8-8ca6-321fa1c1bafc.png", ""
	case "media_image_first_frame_v1":
		return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/first-frame-images/result-01_659b03f1-91e8-4dc5-b051-e10becc6e810.png", ""
	case "media_video_generate_v1":
		if strings.Contains(task.ReferenceTaskName, "video_b") {
			return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/videos/video-01_1a74ce8a-9e5f-4678-a62b-427dfcb7758e.mp4", ""
		}
		return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/videos/video-01_805cbdb8-3ee9-45a9-bbe6-cc2ee5204062.mp4", ""
	case "media_video_clip_v1", "media_content_ai_fill_v1", "media_publish_feishu_v1":
		return base + "/uploads/teams/cf88f650-a530-4356-81db-391b62522778/clips/clip-fcf979fe5dd8144cf1d96ca4d7af3252_alternate-mirror.mp4", `{"title":"浅紫韩风度假美裙，甜到爆表","content":"轻盈浅紫无袖，夏日海边度假甜美减龄，精美边缘装饰！#穿搭 #韩系 #度假风"}`
	default:
		return "", ""
	}
}

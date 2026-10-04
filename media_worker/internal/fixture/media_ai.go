package fixture

import (
	"fmt"
	"net/url"
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

type Executor struct {
	baseURL    string
	resultURLs map[string]string
}

func NewExecutor(baseURL string) (*Executor, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("fixture asset base must be an HTTP(S) URL without credentials, query or fragment")
	}
	return &Executor{baseURL: strings.TrimRight(baseURL, "/"), resultURLs: make(map[string]string)}, nil
}

// SetVideoResultURL points the historical video fixture at a pre-uploaded
// object in the target storage. The Mock Worker never writes to storage or
// uses a User token; it only returns this known result for one video task.
func (e *Executor) SetVideoResultURL(raw string) error {
	return e.SetResultURL("media.video.generate", raw)
}

// SetResultURL configures a known, pre-uploaded result before polling starts.
// A signed query is allowed; credentials/fragments and internal tasks are not.
// The server remains responsible for validating storage ownership and MIME.
func (e *Executor) SetResultURL(capabilityKey, raw string) error {
	supported := false
	for _, capability := range capabilities {
		if capability.Key == capabilityKey {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("fixture result override requires a supported Capability")
	}
	if raw == "" {
		delete(e.resultURLs, capabilityKey)
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("fixture result must be an HTTP(S) storage URL without credentials or fragment")
	}
	e.resultURLs[capabilityKey] = raw
	return nil
}

func (e *Executor) Execute(task gateway.Task) (map[string]any, error) {
	if task.CapabilityVersion != 1 {
		return nil, fmt.Errorf("media_ai fixture does not support capability version %d", task.CapabilityVersion)
	}
	resourceURL, content := e.fixtureResult(task)
	if resourceURL == "" && content == "" {
		return nil, fmt.Errorf("media_ai fixture does not support capability %q", task.CapabilityKey)
	}
	return map[string]any{
		"candidates": []map[string]any{{"index": 0, "url": resourceURL, "content": content, "is_primary": true}},
		"provenance": map[string]any{"profile": "media_ai_fixture", "source": source, "mock": true},
	}, nil
}

func (e *Executor) fixtureResult(task gateway.Task) (string, string) {
	if resultURL := e.resultURLs[task.CapabilityKey]; resultURL != "" {
		return resultURL, ""
	}
	base := e.baseURL
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

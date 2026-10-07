package fixture

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

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
	baseURL           string
	resultURLs        map[string][]string
	providerResults   map[providerTaskRef]ProviderBinding
	failureMu         sync.Mutex
	failureStep       string
	failureCall       int
	failureCounts     map[string]int
	failureDispatches map[string]bool
}

// SetFailureOnce is an explicit test-only fault: fail the Nth distinct
// dispatch for a step in each Job. Re-delivery preserves the same outcome.
// Configure before polling; no internal server task can enter this executor.
func (e *Executor) SetFailureOnce(step string, call int) error {
	if strings.TrimSpace(step) == "" || call < 1 || call > 8 {
		return fmt.Errorf("fixture failure requires a step and call between one and eight")
	}
	e.failureStep, e.failureCall = step, call
	e.failureCounts = make(map[string]int)
	e.failureDispatches = make(map[string]bool)
	return nil
}

func (e *Executor) shouldFail(task gateway.Task) bool {
	step, _ := task.Input["step_key"].(string)
	job, _ := task.Input["job_id"].(string)
	if e.failureStep == "" || step != e.failureStep || job == "" || task.DispatchID == "" {
		return false
	}
	e.failureMu.Lock()
	defer e.failureMu.Unlock()
	key := job + ":" + task.DispatchID
	if failed, exists := e.failureDispatches[key]; exists {
		return failed
	}
	e.failureCounts[job]++
	failed := e.failureCounts[job] == e.failureCall
	e.failureDispatches[key] = failed
	return failed
}

func NewExecutor(baseURL string) (*Executor, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("fixture asset base must be an HTTP(S) URL without credentials, query or fragment")
	}
	return &Executor{baseURL: strings.TrimRight(baseURL, "/"), resultURLs: make(map[string][]string)}, nil
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
	if raw == "" {
		return e.SetResultURLs(capabilityKey, nil)
	}
	return e.SetResultURLs(capabilityKey, []string{raw})
}

// SetResultURLs configures distinct already-stored candidates for a fixture.
// Validate the whole configuration before replacing any existing result.
func (e *Executor) SetResultURLs(capabilityKey string, urls []string) error {
	if e.providerResults != nil {
		return fmt.Errorf("provider fixtures require explicit Job/Step bindings")
	}
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
	if len(urls) == 0 {
		delete(e.resultURLs, capabilityKey)
		return nil
	}
	if err := validateResultURLs(urls); err != nil {
		return err
	}
	e.resultURLs[capabilityKey] = append([]string(nil), urls...)
	return nil
}

func validResultURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Fragment == ""
}

func validateResultURLs(urls []string) error {
	if len(urls) > 8 {
		return fmt.Errorf("fixture supports at most eight candidates")
	}
	seen := map[string]bool{}
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil || !validResultURL(raw) {
			return fmt.Errorf("fixture result must be an HTTP(S) storage URL without credentials or fragment")
		}
		object := u.Scheme + "://" + u.Host + u.EscapedPath()
		if seen[object] {
			return fmt.Errorf("fixture candidates must use distinct storage objects")
		}
		seen[object] = true
	}
	return nil
}

func (e *Executor) Execute(task gateway.Task) (map[string]any, error) {
	if e.providerResults != nil {
		return e.executeProvider(task)
	}
	if task.CapabilityVersion != 1 {
		return nil, fmt.Errorf("media_ai fixture does not support capability version %d", task.CapabilityVersion)
	}
	resourceURL, content := e.fixtureResult(task)
	if resourceURL == "" && content == "" {
		return nil, fmt.Errorf("media_ai fixture does not support capability %q", task.CapabilityKey)
	}
	if e.shouldFail(task) {
		return nil, fmt.Errorf("explicit fixture capability failure")
	}
	candidates := []map[string]any{{"index": 0, "url": resourceURL, "content": content, "is_primary": true}}
	if urls := e.resultURLs[task.CapabilityKey]; len(urls) > 1 {
		candidates = make([]map[string]any, len(urls))
		for i, resultURL := range urls {
			candidates[i] = map[string]any{"index": i, "url": resultURL, "content": "", "is_primary": i == 0}
		}
	}
	return map[string]any{
		"candidates": candidates,
		"provenance": map[string]any{"profile": "media_ai_fixture", "source": source, "mock": true},
	}, nil
}

func (e *Executor) fixtureResult(task gateway.Task) (string, string) {
	if urls := e.resultURLs[task.CapabilityKey]; len(urls) > 0 {
		return urls[0], ""
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

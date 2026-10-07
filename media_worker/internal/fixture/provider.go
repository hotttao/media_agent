package fixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"media_agent/media_worker/internal/gateway"
)

const ProviderProfile = "provider_fixture"

var providerCapabilities = []gateway.Capability{
	{Key: "jimeng.i2i.first_frame_prompt", Version: 1, Concurrency: 1},
	{Key: "jimeng.i2i.multi_image_prompt", Version: 1, Concurrency: 1},
	{Key: "doubao.i2v.first_frame_prompt", Version: 1, Concurrency: 1},
	{Key: "doubao.i2v.multi_image_prompt", Version: 1, Concurrency: 1},
}

// ProviderBinding selects known results by exact Job/Step, not by business
// names hidden inside a Provider implementation. Configure before polling.
type ProviderBinding struct {
	JobID             string   `json:"job_id"`
	StepKey           string   `json:"step_key"`
	CapabilityKey     string   `json:"capability_key"`
	CapabilityVersion int      `json:"capability_version"`
	ResultURLs        []string `json:"result_urls"`
	ResultFiles       []string `json:"result_files"`
}

type providerTaskRef struct{ job, step string }

func ProviderCapabilities() []gateway.Capability {
	return append([]gateway.Capability(nil), providerCapabilities...)
}

// ReadProviderConfig deliberately omits decoder errors: fixture files may
// contain private signed URLs. Unknown fields and trailing data are rejected.
func ReadProviderConfig(r io.Reader) (*Executor, error) {
	var config struct {
		Tasks []ProviderBinding `json:"tasks"`
	}
	data, err := io.ReadAll(io.LimitReader(r, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return nil, errors.New("invalid provider fixture configuration")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid provider fixture configuration")
	}
	return NewProviderExecutor(config.Tasks)
}

func NewProviderExecutor(bindings []ProviderBinding) (*Executor, error) {
	if len(bindings) == 0 || len(bindings) > 256 {
		return nil, errors.New("provider fixture requires one to 256 explicit Job/Step bindings")
	}
	e := &Executor{providerResults: make(map[providerTaskRef]ProviderBinding)}
	for _, binding := range bindings {
		if strings.TrimSpace(binding.JobID) == "" || strings.TrimSpace(binding.StepKey) == "" || binding.JobID != strings.TrimSpace(binding.JobID) || binding.StepKey != strings.TrimSpace(binding.StepKey) || !supportedProvider(binding.CapabilityKey, binding.CapabilityVersion) || (len(binding.ResultURLs) == 0) == (len(binding.ResultFiles) == 0) || len(binding.ResultFiles) > 8 {
			return nil, errors.New("invalid provider fixture Job/Step binding")
		}
		ref := providerTaskRef{binding.JobID, binding.StepKey}
		if _, exists := e.providerResults[ref]; exists {
			return nil, errors.New("duplicate provider fixture Job/Step binding")
		}
		if len(binding.ResultURLs) > 0 {
			if err := validateResultURLs(binding.ResultURLs); err != nil {
				return nil, err
			}
		}
		for _, file := range binding.ResultFiles {
			if strings.TrimSpace(file) == "" {
				return nil, errors.New("invalid provider fixture result file")
			}
		}
		binding.ResultURLs = append([]string(nil), binding.ResultURLs...)
		binding.ResultFiles = append([]string(nil), binding.ResultFiles...)
		e.providerResults[ref] = binding
	}
	return e, nil
}

func supportedProvider(key string, version int) bool {
	for _, cap := range providerCapabilities {
		if cap.Key == key && cap.Version == version {
			return true
		}
	}
	return false
}

// AvailableCapabilities registers only configured contracts. Old business
// capabilities and server-internal tasks never enter the Provider profile.
func (e *Executor) AvailableCapabilities() []gateway.Capability {
	if e.providerResults == nil {
		return Capabilities()
	}
	var available []gateway.Capability
	for _, cap := range providerCapabilities {
		for _, binding := range e.providerResults {
			if binding.CapabilityKey == cap.Key && binding.CapabilityVersion == cap.Version {
				available = append(available, cap)
				break
			}
		}
	}
	return available
}

func (e *Executor) executeProvider(ctx context.Context, task gateway.Task) (map[string]any, error) {
	if !supportedProvider(task.CapabilityKey, task.CapabilityVersion) {
		return nil, errors.New("unsupported provider fixture Capability")
	}
	job, _ := task.Input["job_id"].(string)
	step, _ := task.Input["step_key"].(string)
	binding, ok := e.providerResults[providerTaskRef{job, step}]
	if !ok || binding.CapabilityKey != task.CapabilityKey || binding.CapabilityVersion != task.CapabilityVersion {
		return nil, errors.New("provider fixture task has no matching Job/Step binding")
	}
	prompt, ok := task.Input["prompt"].(string)
	if !ok || strings.TrimSpace(prompt) == "" {
		return nil, errors.New("provider prompt must be a nonempty rendered string")
	}
	var images []string
	switch raw := task.Input["images"].(type) {
	case []string:
		images = raw
	case []any:
		for _, item := range raw {
			image, ok := item.(string)
			if !ok {
				return nil, errors.New("provider images must be an ordered URL array")
			}
			images = append(images, image)
		}
	default:
		return nil, errors.New("provider images must be an ordered URL array")
	}
	if len(images) == 0 || (strings.HasSuffix(task.CapabilityKey, ".first_frame_prompt") && len(images) != 1) {
		return nil, errors.New("provider image count violates Capability contract")
	}
	for _, image := range images {
		if !validResultURL(image) {
			return nil, errors.New("provider image requires an HTTP(S) media URL")
		}
	}
	if e.shouldFail(task) {
		return nil, errors.New("explicit fixture capability failure")
	}
	candidates := make([]map[string]any, len(binding.ResultURLs))
	for i, raw := range binding.ResultURLs {
		candidates[i] = map[string]any{"index": i, "url": raw, "content": "", "is_primary": i == 0}
	}
	if len(binding.ResultFiles) > 0 {
		if e.uploadFile == nil {
			return nil, errors.New("provider fixture file uploader unavailable")
		}
		candidates = make([]map[string]any, len(binding.ResultFiles))
		for i, file := range binding.ResultFiles {
			path, err := e.uploadFile(ctx, task, i, file)
			if err != nil {
				return nil, err
			}
			if path == "" {
				return nil, errors.New("provider fixture upload unconfirmed")
			}
			candidates[i] = map[string]any{"index": i, "object_path": path, "content": "", "is_primary": i == 0}
		}
	}
	return map[string]any{"candidates": candidates, "provenance": map[string]any{"profile": ProviderProfile, "source": source, "mock": true}}, nil
}

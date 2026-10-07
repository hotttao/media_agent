package fixture

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"media_agent/media_worker/internal/gateway"
)

func TestProviderFileResultsAreExplicitAndConfirmed(t *testing.T) {
	b := providerBinding("job", "step", "jimeng.i2i.multi_image_prompt", "https://storage.test/result")
	b.ResultFiles = []string{"/private/result-a.png", "/private/result-b.png"}
	if _, err := NewProviderExecutor([]ProviderBinding{b}); err == nil {
		t.Fatal("ambiguous URL/file binding accepted")
	}
	b.ResultURLs = nil
	e, err := NewProviderExecutor([]ProviderBinding{b})
	if err != nil {
		t.Fatal(err)
	}
	task := providerTask("job", "step", b.CapabilityKey, "https://storage.test/input.png")
	if _, err = e.Execute(task); err == nil {
		t.Fatal("file result accepted without uploader")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	e.SetFileUploader(func(got context.Context, actual gateway.Task, index int, file string) (string, error) {
		calls++
		if got != ctx || actual.DispatchID != task.DispatchID || file != b.ResultFiles[index] {
			t.Error("upload binding changed")
		}
		return "materials/org/scope/file-" + file[len(file)-5:len(file)-4], nil
	})
	out, err := e.ExecuteContext(ctx, task)
	if err != nil || calls != 2 {
		t.Fatal("file results failed", err)
	}
	candidates := out["candidates"].([]map[string]any)
	if len(candidates) != 2 || candidates[0]["object_path"] == "" || candidates[0]["url"] != nil || candidates[1]["is_primary"] != false {
		t.Fatal("uploaded result shape invalid")
	}
	e.SetFileUploader(func(context.Context, gateway.Task, int, string) (string, error) {
		return "", errors.New("upload unconfirmed")
	})
	if out, err = e.Execute(task); err == nil || out != nil {
		t.Fatal("failed upload became completed result")
	}
	if e.SetFailureOnce("step", 1) != nil {
		t.Fatal("failure fixture rejected")
	}
	calls = 0
	e.SetFileUploader(func(context.Context, gateway.Task, int, string) (string, error) { calls++; return "path", nil })
	if _, err = e.Execute(task); err == nil || calls != 0 {
		t.Fatal("explicit generation failure uploaded results")
	}
}

func providerBinding(job, step, key, result string) ProviderBinding {
	return ProviderBinding{JobID: job, StepKey: step, CapabilityKey: key, CapabilityVersion: 1, ResultURLs: []string{result}}
}

func providerTask(job, step, key string, images ...string) gateway.Task {
	return gateway.Task{DispatchID: job + "-" + step, CapabilityKey: key, CapabilityVersion: 1, Input: map[string]any{"job_id": job, "step_key": step, "prompt": "already rendered", "images": images}}
}

func TestProviderFixtureReusesCapabilityWithExactJobStepResults(t *testing.T) {
	key := "jimeng.i2i.multi_image_prompt"
	bindings := []ProviderBinding{
		providerBinding("job-a", "model", key, "https://storage.test/model.png"),
		providerBinding("job-a", "style", key, "https://storage.test/style.png"),
		providerBinding("job-b", "model", key, "https://storage.test/other.png"),
	}
	bindings[0].ResultURLs = append(bindings[0].ResultURLs, "https://storage.test/model2.png")
	e, err := NewProviderExecutor(bindings)
	if err != nil {
		t.Fatal(err)
	}
	bindings[0].ResultURLs[0] = "https://storage.test/changed.png"
	available := e.AvailableCapabilities()
	if len(available) != 1 || available[0].Key != key {
		t.Fatal("registered unconfigured or business capabilities")
	}
	var wg sync.WaitGroup
	for _, test := range []struct{ job, step, url string }{{"job-a", "model", "https://storage.test/model.png"}, {"job-a", "style", "https://storage.test/style.png"}, {"job-b", "model", "https://storage.test/other.png"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task := providerTask(test.job, test.step, key, "https://storage.test/person.png", "https://storage.test/garment.png")
			output, err := e.Execute(task)
			if err != nil {
				t.Error(err)
				return
			}
			candidates := output["candidates"].([]map[string]any)
			if candidates[0]["url"] != test.url || output["provenance"].(map[string]any)["mock"] != true || output["provenance"].(map[string]any)["profile"] != ProviderProfile {
				t.Error("wrong binding or mock provenance")
			}
			if test.job == "job-a" && test.step == "model" && (len(candidates) != 2 || candidates[1]["index"] != 1 || candidates[1]["is_primary"] != false) {
				t.Error("candidate ordering lost")
			}
			if task.Input["prompt"] != "already rendered" || task.Input["images"].([]string)[0] != "https://storage.test/person.png" {
				t.Error("client changed rendered prompt or input image order")
			}
		}()
	}
	wg.Wait()
	if e.SetResultURL(key, "https://storage.test/unscoped.png") == nil {
		t.Fatal("provider accepted business/unscoped override")
	}
	for _, task := range []gateway.Task{
		providerTask("wrong-job", "model", key, "https://storage.test/input.png"),
		providerTask("job-a", "unknown", key, "https://storage.test/input.png"),
		providerTask("job-a", "model", "doubao.i2v.first_frame_prompt", "https://storage.test/input.png"),
		providerTask("job-a", "model", "media.image.model", "https://storage.test/input.png"),
		providerTask("job-a", "model", "media_render_template_v1", "https://storage.test/input.png"),
	} {
		if _, err := e.Execute(task); err == nil {
			t.Fatal("unbound or internal task accepted")
		}
	}
}

func TestAllFourProviderFixturesEnforceFixedInputContract(t *testing.T) {
	for _, cap := range ProviderCapabilities() {
		t.Run(cap.Key, func(t *testing.T) {
			e, err := NewProviderExecutor([]ProviderBinding{providerBinding("job", "step", cap.Key, "https://storage.test/result")})
			if err != nil {
				t.Fatal(err)
			}
			task := providerTask("job", "step", cap.Key, "https://storage.test/input.png")
			if _, err := e.Execute(task); err != nil {
				t.Fatal("one image must be supported", err)
			}
			task.Input["images"] = []any{"https://storage.test/a.png", "https://storage.test/b.png"}
			_, err = e.Execute(task)
			if (err != nil) != strings.HasSuffix(cap.Key, ".first_frame_prompt") {
				t.Fatal("first frame and multi-image counts not distinguished")
			}
			for _, invalid := range []any{nil, "https://storage.test/input.png", []any{}, []any{123}, []any{"file:///tmp/image"}, []any{"https://user:private@storage.test/image"}} {
				task.Input["images"] = invalid
				if _, err := e.Execute(task); err == nil || strings.Contains(err.Error(), "private") {
					t.Fatal("bad image schema accepted or secret exposed")
				}
			}
			task.Input["images"] = []any{"https://storage.test/input.png"}
			for _, prompt := range []any{nil, " ", 42} {
				task.Input["prompt"] = prompt
				if _, err := e.Execute(task); err == nil {
					t.Fatal("bad prompt accepted")
				}
			}
			task.Input["prompt"] = "rendered"
			task.CapabilityVersion = 2
			if _, err := e.Execute(task); err == nil {
				t.Fatal("unknown contract version accepted")
			}
		})
	}
}

func TestProviderConfigRejectsAmbiguityAndDoesNotLeakURLs(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"tasks":[]}`, `{"tasks":[],"unknown":"private"}`, `{} {}`,
		`{"tasks":[{"job_id":"job","step_key":"s","capability_key":"media_render_template_v1","capability_version":1,"result_urls":["https://storage.test/private"]}]}`,
		`{"tasks":[{"job_id":"job","step_key":"s","capability_key":"jimeng.i2i.multi_image_prompt","capability_version":1,"result_urls":["https://user:private@storage.test/result"]}]}`,
	} {
		if _, err := ReadProviderConfig(strings.NewReader(raw)); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("bad private config accepted or leaked")
		}
	}
	b := providerBinding("job", "s", "jimeng.i2i.multi_image_prompt", "https://storage.test/result.png")
	if _, err := NewProviderExecutor([]ProviderBinding{b, b}); err == nil {
		t.Fatal("duplicate binding accepted")
	}
	b.ResultURLs = []string{"https://storage.test/result.png?sig=private", "https://storage.test/result.png?sig=other"}
	if _, err := NewProviderExecutor([]ProviderBinding{b}); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("same object mistaken for distinct candidates")
	}
	const valid = `{"tasks":[{"job_id":"job","step_key":"model","capability_key":"jimeng.i2i.multi_image_prompt","capability_version":1,"result_urls":["https://storage.test/result.png"]}]}`
	if _, err := ReadProviderConfig(strings.NewReader(valid)); err != nil {
		t.Fatal("explicit config rejected", err)
	}
}

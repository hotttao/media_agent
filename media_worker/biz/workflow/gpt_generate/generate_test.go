package gpt_generate

import (
	"context"
	"encoding/json"
	"errors"
	"media_agent/media_worker/biz/domain/gpt"
	"media_agent/media_worker/biz/shared/worker_runtime"
	"reflect"
	"strings"
	"testing"
)

type repoFake struct {
	cp          *gpt.Checkpoint
	saves, fail int
}

func (r *repoFake) Save(_ context.Context, c gpt.Checkpoint) error {
	r.saves++
	if r.saves == r.fail {
		return errors.New("disk")
	}
	b, _ := json.Marshal(c)
	var clone gpt.Checkpoint
	_ = json.Unmarshal(b, &clone)
	r.cp = &clone
	return nil
}
func (r *repoFake) Load(context.Context, string, int) (*gpt.Checkpoint, error) {
	if r.cp == nil {
		return nil, nil
	}
	b, _ := json.Marshal(r.cp)
	var c gpt.Checkpoint
	_ = json.Unmarshal(b, &c)
	return &c, nil
}
func (r *repoFake) Pending(context.Context) ([]gpt.Checkpoint, error) {
	if r.cp == nil || r.cp.Closed {
		return nil, nil
	}
	return []gpt.Checkpoint{*r.cp}, nil
}

type generatorFake struct {
	request          gpt.Request
	submits, fetches int
	created, lost    bool
	human, uncertain bool
}

func (g *generatorFake) Lookup(context.Context, string) (gpt.LookupResult, error) {
	if g.uncertain {
		return gpt.LookupResult{State: gpt.Uncertain}, nil
	}
	if g.created {
		return gpt.LookupResult{State: gpt.Found, Reference: gpt.Reference(g.request.Key)}, nil
	}
	return gpt.LookupResult{State: gpt.Absent}, nil
}
func (g *generatorFake) Submit(_ context.Context, r gpt.Request) (gpt.Reference, error) {
	g.request = r
	g.submits++
	g.created = true
	if g.lost {
		g.lost = false
		return "", errors.New("lost response")
	}
	return gpt.Reference(r.Key), nil
}
func (g *generatorFake) Observe(context.Context, gpt.Reference) (gpt.Observation, error) {
	if g.human {
		return gpt.Observation{State: gpt.NeedsHuman}, nil
	}
	return gpt.Observation{State: gpt.Succeeded, Artifacts: []gpt.Artifact{{ID: "exact-image", MediaType: "image/png"}}}, nil
}
func (g *generatorFake) FetchArtifacts(context.Context, gpt.Reference, []gpt.Artifact) ([]gpt.Download, error) {
	g.fetches++
	return []gpt.Download{{ArtifactID: "exact-image", LocalPath: "/private/result.png", ChecksumSHA256: strings.Repeat("a", 64), SizeBytes: 40}}, nil
}

type gatewayFake struct {
	err     error
	uploads int
}

func (g *gatewayFake) Heartbeat(context.Context, string, worker_runtime.Task) error { return g.err }
func (g *gatewayFake) Register(context.Context, string, []worker_runtime.Capability) error {
	return nil
}
func (g *gatewayFake) Poll(context.Context, string, worker_runtime.Capability) (*worker_runtime.Task, error) {
	panic("not a scheduler")
}
func (g *gatewayFake) Complete(context.Context, string, worker_runtime.Task, string, map[string]any) (*worker_runtime.ResultReceipt, error) {
	panic("delivery belongs to execute")
}
func (g *gatewayFake) FailWithKind(context.Context, string, worker_runtime.Task, string, string, string) (*worker_runtime.ResultReceipt, error) {
	panic("delivery belongs to execute")
}
func (g *gatewayFake) UploadFile(context.Context, string, worker_runtime.Task, int, string) (string, error) {
	g.uploads++
	return "materials/org/media/result.png", nil
}

type filesFake struct {
	keys, urls []string
	bad        bool
}

func (f *filesFake) Fetch(_ context.Context, key, url string) (worker_runtime.Artifact, error) {
	f.keys = append(f.keys, key)
	f.urls = append(f.urls, url)
	return worker_runtime.Artifact{LocalPath: "/private/" + key + ".png", ChecksumSHA256: strings.Repeat("b", 64), SizeBytes: 30}, nil
}
func (f *filesFake) Verify(context.Context, worker_runtime.Artifact) error {
	if f.bad {
		return errors.New("changed")
	}
	return nil
}
func setup() (Workflow, *repoFake, *generatorFake, *gatewayFake, *filesFake, worker_runtime.Task) {
	r := &repoFake{}
	g := &generatorFake{}
	gw := &gatewayFake{}
	f := &filesFake{}
	return Workflow{Generator: g, Checkpoints: r, Gateway: gw, Uploader: gw, Inputs: f, Files: f, WorkerID: "worker", Namespace: "private", Size: "auto", WebModel: "Thinking"}, r, g, gw, f, worker_runtime.Task{DispatchID: "dispatch", Attempt: 1, CapabilityKey: gpt.TextCapability, CapabilityVersion: 1, Input: map[string]any{"prompt": "rendered image prompt"}}
}
func TestGPTTextAndOrderedReferenceImages(t *testing.T) {
	for _, count := range []int{0, 1, 4} {
		w, r, g, gw, f, task := setup()
		if count > 0 {
			task.CapabilityKey = gpt.ReferenceCapability
			var refs []string
			for i := 0; i < count; i++ {
				refs = append(refs, "https://input.example/private-signed-"+string(rune('a'+i)))
			}
			task.Input["images"] = refs
		}
		out, err := w.ExecuteContext(context.Background(), task)
		if err != nil {
			t.Fatal(err)
		}
		if g.submits != 1 || gw.uploads != 1 || len(g.request.Images) != count || g.request.Size != "auto" {
			t.Fatal("wrong generation", g.request)
		}
		if count > 1 && f.keys[0] == f.keys[1] {
			t.Fatal("references overwrite one another")
		}
		for i, image := range g.request.Images {
			if !strings.Contains(image.Path, f.keys[i]) {
				t.Fatal("reference ordering changed")
			}
		}
		raw, _ := json.Marshal(r.cp)
		if strings.Contains(string(raw), "private-signed-") {
			t.Fatal("signed input URL persisted")
		}
		p := out["provenance"].(map[string]any)
		if p["backend"] != "web" || p["provider"] != "gpt" {
			t.Fatal(p)
		}
	}
}
func TestGPTInvalidInputsDoNotStartGeneration(t *testing.T) {
	for _, refs := range []any{[]string{}, []string{"1", "2", "3", "4", "5"}, []any{42}, "wrong"} {
		w, _, g, gw, _, task := setup()
		task.CapabilityKey = gpt.ReferenceCapability
		task.Input["images"] = refs
		if _, err := w.ExecuteContext(context.Background(), task); err == nil || errors.Is(err, worker_runtime.ErrExecutionPending) || g.submits != 0 || gw.uploads != 0 {
			t.Fatal("invalid input accepted", refs, err)
		}
	}
}
func TestGPTLostResponseAndCheckpointFailuresNeverRepeatKnownSubmission(t *testing.T) {
	for _, stage := range []int{0, 1, 2, 3, 4} {
		w, r, g, _, _, task := setup()
		if stage == 0 {
			g.lost = true
		} else {
			r.fail = stage
		}
		_, err := w.ExecuteContext(context.Background(), task)
		if !errors.Is(err, worker_runtime.ErrExecutionPending) {
			t.Fatal(stage, err)
		}
		tasks, _ := w.PendingTasks(context.Background())
		if len(tasks) > 0 {
			task = tasks[0]
			if task.Input != nil {
				t.Fatal("recovery included signed inputs")
			}
		}
		_, err = w.ExecuteContext(context.Background(), task)
		if err != nil || g.submits != 1 {
			t.Fatal(stage, err, g.submits)
		}
	}
}
func TestGPTUnknownSubmissionAndHumanStateStayPending(t *testing.T) {
	for _, human := range []bool{false, true} {
		w, _, g, gw, _, task := setup()
		g.human = human
		g.uncertain = !human
		if _, err := w.ExecuteContext(context.Background(), task); !errors.Is(err, worker_runtime.ErrExecutionPending) || gw.uploads != 0 {
			t.Fatal(err)
		}
		if !human && g.submits != 0 {
			t.Fatal("uncertain lookup authorized resubmission")
		}
	}
}
func TestGPTRecoveryRechecksEligibilityAndFileIntegrity(t *testing.T) {
	w, r, g, gw, f, task := setup()
	g.human = true
	_, _ = w.ExecuteContext(context.Background(), task)
	gw.err = worker_runtime.ErrResultRejected
	if _, err := w.ExecuteContext(context.Background(), task); !errors.Is(err, worker_runtime.ErrExecutionAbandoned) || !r.cp.Closed || gw.uploads != 0 {
		t.Fatal(err)
	}
	w, _, g, gw, f, task = setup()
	f.bad = true
	if _, err := w.ExecuteContext(context.Background(), task); !errors.Is(err, worker_runtime.ErrExecutionPending) || gw.uploads != 0 {
		t.Fatal("changed file uploaded", err)
	}
}
func TestGPTAttemptAndNamespaceHaveIndependentKeys(t *testing.T) {
	var keys []string
	for _, v := range []struct {
		attempt int
		ns      string
	}{{1, "one"}, {2, "one"}, {1, "two"}} {
		w, _, g, _, _, task := setup()
		w.Namespace = v.ns
		task.Attempt = v.attempt
		_, err := w.ExecuteContext(context.Background(), task)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, g.request.Key)
	}
	if keys[0] == keys[1] || keys[0] == keys[2] || reflect.DeepEqual(keys[1], keys[2]) {
		t.Fatal(keys)
	}
}

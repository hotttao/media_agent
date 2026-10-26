package doubao_generate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"media_agent/media_worker/biz/domain/doubao"
	"media_agent/media_worker/biz/shared/worker_runtime"
)

type checkpointFake struct {
	data     *doubao.Checkpoint
	failSave int
	saves    int
	events   *[]string
}

func clone(c doubao.Checkpoint) doubao.Checkpoint {
	raw, _ := json.Marshal(c)
	var out doubao.Checkpoint
	_ = json.Unmarshal(raw, &out)
	return out
}
func (s *checkpointFake) Save(_ context.Context, c doubao.Checkpoint) error {
	s.saves++
	*s.events = append(*s.events, "save")
	if s.saves == s.failSave {
		return errors.New("disk unavailable")
	}
	v := clone(c)
	s.data = &v
	return nil
}
func (s *checkpointFake) Load(context.Context, string, int) (*doubao.Checkpoint, error) {
	if s.data == nil {
		return nil, nil
	}
	v := clone(*s.data)
	return &v, nil
}
func (s *checkpointFake) Pending(context.Context) ([]doubao.Checkpoint, error) {
	if s.data == nil || s.data.Closed {
		return nil, nil
	}
	return []doubao.Checkpoint{clone(*s.data)}, nil
}

type generatorFake struct {
	created                    bool
	submits, observes, fetches int
	lookupState                doubao.LookupState
	submitLost                 bool
	events                     *[]string
	requests                   []doubao.Request
}

func (g *generatorFake) Lookup(context.Context, string) (doubao.LookupResult, error) {
	*g.events = append(*g.events, "lookup")
	if g.lookupState != "" {
		return doubao.LookupResult{State: g.lookupState}, nil
	}
	if g.created {
		return doubao.LookupResult{State: doubao.Found, Reference: "exact-reference"}, nil
	}
	return doubao.LookupResult{State: doubao.Absent}, nil
}
func (g *generatorFake) Submit(_ context.Context, r doubao.Request) (doubao.Reference, error) {
	*g.events = append(*g.events, "submit")
	g.submits++
	g.created = true
	g.requests = append(g.requests, r)
	if g.submitLost {
		g.submitLost = false
		return "", errors.New("response lost")
	}
	return "exact-reference", nil
}
func (g *generatorFake) Observe(context.Context, doubao.Reference) (doubao.Observation, error) {
	g.observes++
	*g.events = append(*g.events, "observe")
	return doubao.Observation{State: doubao.Succeeded, Artifacts: []doubao.Artifact{{ID: "exact-artifact", Kind: doubao.Video}}}, nil
}
func (g *generatorFake) FetchArtifacts(_ context.Context, _ doubao.Reference, as []doubao.Artifact) ([]doubao.Download, error) {
	g.fetches++
	*g.events = append(*g.events, "fetch")
	if len(as) != 1 || as[0].ID != "exact-artifact" {
		return nil, errors.New("wrong artifact")
	}
	return []doubao.Download{{ArtifactID: "exact-artifact", LocalPath: "/private/exact.mp4", ChecksumSHA256: strings.Repeat("a", 64), SizeBytes: 24}}, nil
}

type gatewayFake struct {
	err        error
	events     *[]string
	uploads    int
	uploadLost bool
}

func (g *gatewayFake) Heartbeat(context.Context, string, worker_runtime.Task) error {
	*g.events = append(*g.events, "eligibility")
	return g.err
}
func (g *gatewayFake) Register(context.Context, string, []worker_runtime.Capability) error {
	return nil
}
func (g *gatewayFake) Poll(context.Context, string, worker_runtime.Capability) (*worker_runtime.Task, error) {
	panic("generation must not poll")
}
func (g *gatewayFake) Complete(context.Context, string, worker_runtime.Task, string, map[string]any) (*worker_runtime.ResultReceipt, error) {
	panic("generation must not bypass result journal")
}
func (g *gatewayFake) FailWithKind(context.Context, string, worker_runtime.Task, string, string, string) (*worker_runtime.ResultReceipt, error) {
	panic("generation must not report uncertain side effects as failure")
}
func (g *gatewayFake) UploadFile(_ context.Context, _ string, t worker_runtime.Task, index int, file string) (string, error) {
	g.uploads++
	*g.events = append(*g.events, "upload")
	if index != 0 || file != "/private/exact.mp4" || t.DispatchID != "dispatch" {
		panic("upload identity drift")
	}
	if g.uploadLost {
		g.uploadLost = false
		return "", errors.New("PUT response lost")
	}
	return "materials/org/video", nil
}

type filesFake struct {
	err       error
	downloads int
	events    *[]string
}

func (f *filesFake) Fetch(context.Context, string, string) (worker_runtime.Artifact, error) {
	f.downloads++
	*f.events = append(*f.events, "input")
	return worker_runtime.Artifact{ID: "image", LocalPath: "/private/input.png", ChecksumSHA256: strings.Repeat("b", 64), SizeBytes: 12}, nil
}
func (f *filesFake) Verify(context.Context, worker_runtime.Artifact) error { return f.err }
func setup(t *testing.T) (*Workflow, *checkpointFake, *generatorFake, *gatewayFake, *filesFake, *[]string, worker_runtime.Task) {
	t.Helper()
	events := []string{}
	store := &checkpointFake{events: &events}
	gen := &generatorFake{events: &events}
	gw := &gatewayFake{events: &events}
	files := &filesFake{events: &events}
	flow := &Workflow{Generator: gen, Gateway: gw, Uploader: gw, Checkpoints: store, Inputs: files, Files: files, WorkerID: "worker", Namespace: "credential-hash", Model: "seedance-2.0-fast", DurationSeconds: 5, AspectRatio: "9:16"}
	task := worker_runtime.Task{DispatchID: "dispatch", Attempt: 1, CapabilityKey: CapabilityKey, CapabilityVersion: 1, Input: map[string]any{"prompt": "rendered prompt", "images": []any{"https://storage/input?private-signed-url"}}}
	return flow, store, gen, gw, files, &events, task
}
func TestGenerationPersistsBeforeSideEffectsAndDoesNotKeepSignedURLs(t *testing.T) {
	flow, store, gen, gw, files, events, task := setup(t)
	output, err := flow.ExecuteContext(context.Background(), task)
	if err != nil || output["candidates"] == nil {
		t.Fatal(output, err)
	}
	want := []string{"eligibility", "input", "save", "eligibility", "lookup", "submit", "save", "eligibility", "observe", "eligibility", "fetch", "save", "eligibility", "upload", "save"}
	if !reflect.DeepEqual(*events, want) {
		t.Fatalf("ordering=%v", *events)
	}
	raw, _ := json.Marshal(store.data)
	if strings.Contains(string(raw), "private-signed-url") || store.data.Task.Input != nil {
		t.Fatal("signed input persisted")
	}
	if len(gen.requests) != 1 || gen.requests[0].Kind != doubao.Video || len(gen.requests[0].Images) != 1 || gw.uploads != 1 || files.downloads != 1 {
		t.Fatal("generation contract changed")
	}
}
func TestRestartAcrossEveryCheckpointCommitKeepsOneGeneration(t *testing.T) {
	for _, save := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprint(save), func(t *testing.T) {
			flow, store, gen, _, _, _, task := setup(t)
			store.failSave = save
			_, err := flow.ExecuteContext(context.Background(), task)
			if !errors.Is(err, worker_runtime.ErrExecutionPending) {
				t.Fatalf("expected pending at save %d: %v", save, err)
			}
			store.failSave = 0
			resumed := task
			if store.data != nil {
				resumed.Input = nil
			}
			output, err := flow.ExecuteContext(context.Background(), resumed)
			if err != nil || output["candidates"] == nil || gen.submits != 1 {
				t.Fatalf("resume output=%v err=%v submits=%d", output, err, gen.submits)
			}
		})
	}
}
func TestLostSubmitOrUploadResponseDoesNotResubmit(t *testing.T) {
	for _, step := range []string{"submit", "upload"} {
		t.Run(step, func(t *testing.T) {
			flow, _, gen, gw, _, _, task := setup(t)
			if step == "submit" {
				gen.submitLost = true
			} else {
				gw.uploadLost = true
			}
			if _, err := flow.ExecuteContext(context.Background(), task); !errors.Is(err, worker_runtime.ErrExecutionPending) {
				t.Fatal(err)
			}
			task.Input = nil
			if _, err := flow.ExecuteContext(context.Background(), task); err != nil || gen.submits != 1 {
				t.Fatalf("uncertain side effect regenerated: %v %d", err, gen.submits)
			}
		})
	}
}
func TestUncertainLookupAndRevokedEligibilityNeverSubmit(t *testing.T) {
	flow, store, gen, gw, _, events, task := setup(t)
	gen.lookupState = doubao.Uncertain
	if _, err := flow.ExecuteContext(context.Background(), task); !errors.Is(err, worker_runtime.ErrExecutionPending) || gen.submits != 0 {
		t.Fatal("uncertain lookup submitted")
	}
	*events = nil
	gw.err = worker_runtime.ErrResultRejected
	task.Input = nil
	if _, err := flow.ExecuteContext(context.Background(), task); !errors.Is(err, worker_runtime.ErrExecutionAbandoned) || !store.data.Closed || gen.submits != 0 {
		t.Fatal("revoked execution continued", err)
	}
	if !reflect.DeepEqual(*events, []string{"eligibility", "save"}) {
		t.Fatal("generator used before eligibility", *events)
	}
}
func TestRejectsMultipleImagesAndSeparatesNewAttemptIdentity(t *testing.T) {
	flow, store, gen, _, _, _, task := setup(t)
	task.Input["images"] = []any{"a", "b"}
	if _, err := flow.ExecuteContext(context.Background(), task); err == nil || store.data != nil || gen.submits != 0 {
		t.Fatal("multiple images accepted")
	}
	task.Input["images"] = []any{"one"}
	_, _ = flow.ExecuteContext(context.Background(), task)
	first := gen.requests[0].Key
	store.data = nil
	gen.created = false
	task.Attempt = 2
	_, _ = flow.ExecuteContext(context.Background(), task)
	if gen.requests[1].Key == first {
		t.Fatal("new attempt reused old generation key")
	}
}
func TestChangedManifestFileCannotUpload(t *testing.T) {
	flow, _, gen, gw, files, _, task := setup(t)
	files.err = errors.New("changed")
	if _, err := flow.ExecuteContext(context.Background(), task); !errors.Is(err, worker_runtime.ErrExecutionPending) || gen.submits != 0 || gw.uploads != 0 {
		t.Fatal("changed input submitted")
	}
}

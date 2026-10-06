package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureTemplatesUseSourcePromptsAndOrderedResources(t *testing.T) {
	dir := t.TempDir()
	prompts := map[string]string{
		"03_模特图.md":     "图片一是模特，图片二是服装",
		"04_定妆图.md":     "图片一是模特，图片二是姿势",
		"05_首帧图.md":     "图片一是模特，图片二是场景",
		"06_文生视频提示词.md": "{{ 动作细节 }}",
	}
	for name, body := range prompts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	specs, err := fixtureTemplates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 4 {
		t.Fatalf("got %d templates, want 4", len(specs))
	}
	if specs[0].Prompt != prompts["03_模特图.md"] || specs[1].Prompt != prompts["04_定妆图.md"] || specs[2].Prompt != prompts["05_首帧图.md"] {
		t.Fatal("image prompts must retain exact source bodies")
	}
	if specs[3].Prompt != "{{.movement.content}}" {
		t.Fatalf("video prompt = %q", specs[3].Prompt)
	}
	resources := specs[0].Invocation["resources"].(map[string]any)["items"].([]map[string]any)
	if resources[0]["slot"] != "virtual_ip" || resources[1]["slot"] != "product" {
		t.Fatalf("model image order = %v", resources)
	}
	videoResources := specs[3].Invocation["resources"].(map[string]any)["items"].([]map[string]any)
	if len(videoResources) != 1 || videoResources[0]["slot"] != "first_frame" {
		t.Fatalf("text-only movement must not become a Capability resource: %v", videoResources)
	}
}

func TestWorkflowMigrationStopsAtVideo(t *testing.T) {
	var workflow map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/workflows" {
			if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
				t.Errorf("decode workflow: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"workflow-1","version":1,"status":2,"publication_status":"registered"}`))
	}))
	defer server.Close()
	c := &client{config: config{baseURL: server.URL, scope: "ai_clothing", workflowName: "republished sample"}, http: server.Client()}
	templates := map[string]resourceRef{
		"model": {ID: "model", Version: 1}, "style": {ID: "style", Version: 1},
		"first_frame": {ID: "first_frame", Version: 1}, "video": {ID: "video", Version: 1},
	}
	if _, err := c.createAndPublishWorkflow(context.Background(), templates); err != nil {
		t.Fatal(err)
	}
	steps := workflow["steps"].([]any)
	if workflow["name"] != "republished sample" {
		t.Fatal("migration must use the requested new name, preserving old publication")
	}
	if len(steps) != 4 || workflow["output_step_key"] != "video" {
		t.Fatalf("workflow must stop at video: %v", workflow)
	}
}

func TestWorkflowMigrationObservesAcceptedPublicationWithoutRepeating(t *testing.T) {
	creates, publishes, reads := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workflows":
			creates++
			_, _ = w.Write([]byte(`{"id":"workflow-1","version":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workflows/workflow-1/publish":
			publishes++
			w.WriteHeader(202)
			_, _ = w.Write([]byte(`{"id":"workflow-1","version":1,"status":4,"publication_status":"pending"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workflows/workflow-1":
			reads++
			_, _ = w.Write([]byte(`{"id":"workflow-1","version":1,"status":2,"publication_status":"registered"}`))
		default:
			t.Error("unexpected publication request")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c := &client{config: config{baseURL: server.URL, scope: "ai_clothing"}, http: server.Client()}
	result, err := c.createAndPublishWorkflow(context.Background(), map[string]resourceRef{})
	if err != nil || result.ID != "workflow-1" || creates != 1 || publishes != 1 || reads != 1 {
		t.Fatal("asynchronous observation recreated or republished the source")
	}
}

func TestSessionCookieFromState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage-state.json")
	if err := os.WriteFile(path, []byte(`{"cookies":[{"name":"ory_kratos_session","value":"test-session"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := sessionCookieFromState(path, "ory_kratos_session")
	if err != nil || got != "test-session" {
		t.Fatalf("cookie = %q, error = %v", got, err)
	}
}

func TestTemplateMigrationUpdatesPublishedVersionWithoutDuplicating(t *testing.T) {
	updates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/templates":
			_, _ = w.Write([]byte(`{"items":[{"id":"tpl-1","version":1,"name":"model","prompt_body":"old","output_asset_kind":"model_image","capability_key":"media.image.model","status":2}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/templates/tpl-1":
			updates++
			_, _ = w.Write([]byte(`{"version":2}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	c := &client{config: config{baseURL: server.URL, scope: "ai_clothing"}, http: server.Client()}
	ref, err := c.createAndPublishTemplate(context.Background(), templateSpec{
		Name: "model", Prompt: "new", OutputAssetKind: "model_image", CapabilityKey: "media.image.model",
	})
	if err != nil || ref.ID != "tpl-1" || ref.Version != 2 || updates != 1 {
		t.Fatalf("ref = %+v, updates = %d, error = %v", ref, updates, err)
	}
}

func TestInvocationChangePublishesNewTemplateVersion(t *testing.T) {
	updates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/templates":
			_, _ = w.Write([]byte(`{"items":[{"id":"tpl-video","version":1,"name":"video","prompt_body":"{{.movement.content}}","output_asset_kind":"ai_video","capability_key":"media.video.generate","status":2,"invocation_mapping":{"prompt":{"op":"render_template"},"resources":{"op":"list","items":[{"op":"asset_ref","slot":"first_frame"},{"op":"asset_ref","slot":"movement"}]}}}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/templates/tpl-video":
			updates++
			_, _ = w.Write([]byte(`{"id":"tpl-video","version":2}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	c := &client{config: config{baseURL: server.URL, scope: "ai_clothing"}, http: server.Client()}
	ref, err := c.createAndPublishTemplate(context.Background(), templateSpec{
		Name: "video", Prompt: "{{.movement.content}}", OutputAssetKind: "ai_video", CapabilityKey: "media.video.generate",
		Invocation: map[string]any{"prompt": map[string]any{"op": "render_template"}, "resources": map[string]any{"op": "list", "items": []map[string]any{{"op": "asset_ref", "slot": "first_frame"}}}},
	})
	if err != nil || ref.Version != 2 || updates != 1 {
		t.Fatalf("changed invocation was silently reused: ref=%+v updates=%d err=%v", ref, updates, err)
	}
}

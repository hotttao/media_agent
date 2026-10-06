package worker_gateway

import (
	"context"
	"encoding/json"
	"errors"
	jobdomain "media_agent/media_workflow/biz/domain/job"
	"strings"
	"testing"
	"time"

	"media_agent/media_workflow/biz/shared/client"
)

func TestNativePollResignsWithoutMutatingDurableInput(t *testing.T) {
	now := time.Now()
	flow, repo, engine := newPollFlow(now)
	ref := "object_path:materials/org-1/image.png"
	engine.task.Input["images"] = []any{ref}
	engine.task.Input["prompt"] = "gen_is_plain_text"
	flow.ImportedAssets = importedAssetStub{object: client.ResolvedStorageObject{URL: "https://storage.test/image.png?fresh=1", ExpiresAt: now.Add(time.Hour).Unix()}}
	cmd := PollCommand{Principal: Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, WorkerInstanceID: "desktop-1", CapabilityKey: "jimeng.image", CapabilityVersion: 1}
	first, err := flow.Poll(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first.InputJSON), "fresh=1") || strings.Contains(string(repo.created.InputJSON), "fresh=") || engine.task.Input["images"].([]any)[0] != ref {
		t.Fatal("delivery did not resolve or mutated durable input")
	}
	var delivered map[string]any
	if json.Unmarshal(first.InputJSON, &delivered) != nil || delivered["prompt"] != "gen_is_plain_text" {
		t.Fatal("native prompt was treated as historical Asset")
	}
	repo.active = repo.created
	engine.pollType = ""
	flow.ImportedAssets = importedAssetStub{err: errors.New("storage unavailable")}
	if got, err := flow.Poll(context.Background(), cmd); err == nil || got != nil || engine.pollType != "" || repo.createCalls != 1 {
		t.Fatal("failed redelivery consumed another task or delivered stale input")
	}
	flow.ImportedAssets = importedAssetStub{object: client.ResolvedStorageObject{URL: "https://storage.test/image.png?fresh=2", ExpiresAt: now.Add(time.Hour).Unix()}}
	second, err := flow.Poll(context.Background(), cmd)
	if err != nil || second.ID != first.ID || !strings.Contains(string(second.InputJSON), "fresh=2") || strings.Contains(string(repo.active.InputJSON), "fresh=") || repo.createCalls != 1 {
		t.Fatal("lease redelivery failed to refresh without replacing its snapshot", err)
	}
}

func TestUpstreamStorageURLIsResignedOnlyWithinJobOrganization(t *testing.T) {
	now := time.Now()
	flow := &Flow{StorageOrigin: "https://storage.test", ImportedAssets: importedAssetStub{object: client.ResolvedStorageObject{URL: "https://storage.test/media/materials/org/frame.png?fresh=1", ExpiresAt: now.Add(time.Hour).Unix()}}}
	job := &jobdomain.Job{OrganizationID: "org"}
	old := "https://storage.test/media/materials/org/frame.png?expired_signature=old"
	input := map[string]any{"images": []any{old}}
	resolved, err := flow.resolveInputRefs(context.Background(), job, input)
	if err != nil || resolved["images"].([]any)[0] != "https://storage.test/media/materials/org/frame.png?fresh=1" || input["images"].([]any)[0] != old {
		t.Fatal("upstream storage URL was not safely refreshed", err)
	}
	for _, raw := range []string{"https://storage.test/media/materials/foreign/frame.png", "https://user:secret@storage.test/media/materials/org/frame.png", "https://storage.test/media/materials/org/../foreign/frame.png"} {
		if _, err := flow.resolveInputRefs(context.Background(), job, map[string]any{"images": []any{raw}}); !errors.Is(err, ErrForbidden) {
			t.Fatal("untrusted storage reference was refreshed", err)
		}
	}
}

func TestRenderedPromptResolvesFrozenURLWithoutClientRendering(t *testing.T) {
	ref := "object_path:materials/org/frame.png"
	job := &jobdomain.Job{OrganizationID: "org", Input: jobdomain.JobInput{Materials: []jobdomain.MaterialData{{Name: "scene", URL: ref}}}}
	flow := &Flow{ImportedAssets: importedAssetStub{object: client.ResolvedStorageObject{URL: "https://storage.test/frame.png?fresh=1", ExpiresAt: time.Now().Add(time.Hour).Unix()}}}
	prompt := "Use scene " + ref + ". Keep gen_literal and https://documentation.test unchanged."
	input := map[string]any{"prompt": prompt}
	result, err := flow.resolveInputRefs(context.Background(), job, input)
	if err != nil || result["prompt"] != "Use scene https://storage.test/frame.png?fresh=1. Keep gen_literal and https://documentation.test unchanged." || input["prompt"] != prompt {
		t.Fatal("server-side prompt delivery changed unrelated text or durable prompt", err)
	}
	input["prompt"] = ref + " is the scene."
	result, err = flow.resolveInputRefs(context.Background(), job, input)
	if err != nil || result["prompt"] != "https://storage.test/frame.png?fresh=1 is the scene." {
		t.Fatal("prompt beginning with a reference was interpreted as one object path", err)
	}
	flow.ImportedAssets = importedAssetStub{err: errors.New("storage unavailable")}
	if _, err := flow.resolveInputRefs(context.Background(), job, input); err == nil {
		t.Fatal("prompt-only media reference bypassed storage verification")
	}
}

func TestNativePollDoesNotCreateDispatchWhenStorageIsUnavailable(t *testing.T) {
	flow, repo, engine := newPollFlow(time.Now())
	engine.task.Input["images"] = []any{"object_path:materials/org-1/image.png"}
	flow.ImportedAssets = importedAssetStub{err: errors.New("storage unavailable")}
	got, err := flow.Poll(context.Background(), PollCommand{Principal: Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, WorkerInstanceID: "desktop-1", CapabilityKey: "jimeng.image", CapabilityVersion: 1})
	if err == nil || got != nil || repo.createCalls != 0 {
		t.Fatal("unresolved input was durably delivered")
	}
}

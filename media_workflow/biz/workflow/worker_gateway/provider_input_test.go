package worker_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	capability "media_agent/media_workflow/biz/domain/capability"
	"media_agent/media_workflow/biz/shared/client"
)

func inputContract(firstFrame bool) *capability.Capability {
	kind, task, max := capability.MultiImagePrompt, "jimeng_i2i_multi_image_v1", 0
	if firstFrame {
		kind, task, max = capability.FirstFramePrompt, "jimeng_i2i_first_frame_v1", 1
	}
	key, _ := capability.ContractKey("jimeng", capability.ImageToImage, kind, capability.ExecutionTargetPersonalWorker)
	return &capability.Capability{Key: key, Version: 1, Provider: "jimeng", Mode: "image_generation", GenerationMethod: capability.ImageToImage,
		GenerationType: kind, OutputMediaType: capability.MediaImage, ConductorTaskDef: task, ExecutionTarget: capability.ExecutionTargetPersonalWorker, Status: "published",
		Inputs: []capability.Input{{Name: "prompt", Type: capability.InputString, Required: true}, {Name: "images", Type: capability.InputResourceRefList, Required: true, MediaType: capability.MediaImage, MinItems: 1, MaxItems: max}}}
}

func TestProviderInputDeliveryValidatesContractOnPollAndRedelivery(t *testing.T) {
	now := time.Now()
	flow, dispatches, engine := newPollFlow(now)
	contract := inputContract(false)
	flow.Capabilities = &capabilityRepoStub{item: contract}
	flow.ImportedAssets = importedAssetStub{object: client.ResolvedStorageObject{URL: "https://storage.test/fresh.png", ExpiresAt: now.Add(time.Hour).Unix(), ContentType: "image/png", SizeBytes: 100}}
	engine.task.Input["capability_key"], engine.task.Input["capability_version"] = contract.Key, 1
	engine.task.Input["prompt"] = "rendered prompt"
	engine.task.Input["images"] = []any{"object_path:materials/org-1/a.png", "object_path:materials/org-1/b.png"}
	cmd := PollCommand{Principal: Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, WorkerInstanceID: "desktop-1", CapabilityKey: contract.Key, CapabilityVersion: 1}
	delivered, err := flow.Poll(context.Background(), cmd)
	if err != nil || delivered == nil || dispatches.createCalls != 1 {
		t.Fatal("valid Provider task was not delivered", err)
	}
	var frozen map[string]any
	if json.Unmarshal(dispatches.created.InputJSON, &frozen) != nil || frozen["images"].([]any)[0] != "object_path:materials/org-1/a.png" {
		t.Fatal("input order or durable references changed")
	}
	dispatches.active = dispatches.created
	frozen["images"] = "not an array"
	dispatches.active.InputJSON, _ = json.Marshal(frozen)
	engine.pollType = ""
	if _, err := flow.Poll(context.Background(), cmd); !errors.Is(err, ErrInvalidTask) || engine.pollType != "" || dispatches.createCalls != 1 {
		t.Fatal("invalid redelivery consumed another task or bypassed schema", err)
	}
}

func TestProviderPollRejectsMalformedInputBeforeCreatingDispatch(t *testing.T) {
	for _, scenario := range []string{"missing_prompt", "wrong_prompt", "empty_images", "wrong_images", "first_frame_multiple", "wrong_key", "wrong_version"} {
		t.Run(scenario, func(t *testing.T) {
			flow, dispatches, engine := newPollFlow(time.Now())
			contract := inputContract(true)
			flow.Capabilities = &capabilityRepoStub{item: contract}
			input := engine.task.Input
			input["capability_key"], input["capability_version"] = contract.Key, 1
			input["prompt"], input["images"] = "rendered", []any{"object_path:materials/org-1/a.png"}
			switch scenario {
			case "missing_prompt":
				delete(input, "prompt")
			case "wrong_prompt":
				input["prompt"] = 42
			case "empty_images":
				input["images"] = []any{}
			case "wrong_images":
				input["images"] = []any{42}
			case "first_frame_multiple":
				input["images"] = []any{"a", "b"}
			case "wrong_key":
				input["capability_key"] = "media.image.model"
			case "wrong_version":
				input["capability_version"] = 2
			}
			_, err := flow.Poll(context.Background(), PollCommand{Principal: Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, WorkerInstanceID: "desktop-1", CapabilityKey: contract.Key, CapabilityVersion: 1})
			if !errors.Is(err, ErrInvalidTask) || dispatches.createCalls != 0 {
				t.Fatal("malformed Provider input durably delivered", err)
			}
		})
	}
}

func TestProviderPollVerifiesControlledObjectMediaAndExpiry(t *testing.T) {
	for _, scenario := range []string{"video", "unknown_mime", "malformed_mime", "empty_object", "expired", "external_url", "wrong_org", "valid_storage_url"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Now()
			flow, dispatches, engine := newPollFlow(now)
			contract := inputContract(true)
			flow.Capabilities = &capabilityRepoStub{item: contract}
			flow.StorageOrigin = "https://storage.test"
			object := client.ResolvedStorageObject{URL: "https://storage.test/media/materials/org-1/image.png?fresh=1", ExpiresAt: now.Add(time.Hour).Unix(), ContentType: "image/png", SizeBytes: 100}
			ref := "object_path:materials/org-1/image.png"
			switch scenario {
			case "video":
				object.ContentType = "video/mp4"
			case "unknown_mime":
				object.ContentType = ""
			case "malformed_mime":
				object.ContentType = "image/"
			case "empty_object":
				object.SizeBytes = 0
			case "expired":
				object.ExpiresAt = now.Add(10 * time.Second).Unix()
			case "external_url":
				ref = "https://untrusted.test/image.png"
			case "wrong_org":
				ref = "object_path:materials/foreign/image.png"
			case "valid_storage_url":
				ref = "https://storage.test/media/materials/org-1/image.png?expired=1"
			}
			flow.ImportedAssets = importedAssetStub{object: object}
			engine.task.Input["capability_key"], engine.task.Input["capability_version"] = contract.Key, 1
			engine.task.Input["prompt"], engine.task.Input["images"] = "rendered", []any{ref}
			cmd := PollCommand{Principal: Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}, WorkerInstanceID: "desktop-1", CapabilityKey: contract.Key, CapabilityVersion: 1}
			got, err := flow.Poll(context.Background(), cmd)
			if scenario != "valid_storage_url" {
				if err == nil || got != nil || dispatches.createCalls != 0 {
					t.Fatal("untrusted or incompatible media delivered")
				}
				return
			}
			if err != nil || got == nil || dispatches.createCalls != 1 {
				t.Fatal("controlled storage URL not refreshed", err)
			}
			dispatches.active = dispatches.created
			object.ContentType = "video/mp4"
			flow.ImportedAssets = importedAssetStub{object: object}
			engine.pollType = ""
			if _, err := flow.Poll(context.Background(), cmd); !errors.Is(err, ErrInvalidTask) || engine.pollType != "" || dispatches.createCalls != 1 {
				t.Fatal("redelivery bypassed fresh MIME verification", err)
			}
		})
	}
}

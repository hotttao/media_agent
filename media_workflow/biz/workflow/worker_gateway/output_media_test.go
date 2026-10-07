package worker_gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"media_agent/media_workflow/biz/shared/client"
)

func TestMediaCompletionChecksObjectBeforeNativeUpdate(t *testing.T) {
	for _, scenario := range []string{"valid", "legacy_url", "foreign_org", "foreign_scope", "text", "wrong_type", "empty", "oversize", "external_url", "unordered", "too_many", "storage_unavailable", "string_index", "fractional_index", "string_primary", "number_content", "number_url"} {
		t.Run(scenario, func(t *testing.T) {
			f, _, upload, engine := artifactFixture(t)
			f.StorageOrigin = "https://storage.test"
			resolved := client.ResolvedStorageObject{URL: "https://storage.test/media/materials/org-1/ai_clothing/result.png", ExpiresAt: time.Now().Add(time.Minute).Unix(), ContentType: "image/png", SizeBytes: 256}
			candidate := map[string]any{"index": 0, "object_path": "materials/org-1/ai_clothing/result.png", "is_primary": true}
			candidates := []any{candidate}
			var storageErr error
			switch scenario {
			case "string_index":
				candidate["index"] = "0"
			case "fractional_index":
				candidate["index"] = 0.5
			case "string_primary":
				candidate["is_primary"] = "true"
			case "number_content":
				candidate["content"] = 123
			case "number_url":
				candidate["url"] = 123
			case "legacy_url":
				delete(candidate, "object_path")
				candidate["url"] = resolved.URL
			case "foreign_org":
				candidate["object_path"] = "materials/other/ai_clothing/result.png"
			case "foreign_scope":
				candidate["object_path"] = "materials/org-1/other/result.png"
			case "text":
				delete(candidate, "object_path")
				candidate["content"] = "not an image"
			case "wrong_type":
				resolved.ContentType = "video/mp4"
			case "empty":
				resolved.SizeBytes = 0
			case "oversize":
				resolved.SizeBytes = 17 << 20
			case "external_url":
				delete(candidate, "object_path")
				candidate["url"] = "https://external.test/image.png"
			case "unordered":
				candidate["index"] = 1
			case "too_many":
				for range 8 {
					candidates = append(candidates, candidate)
				}
			case "storage_unavailable":
				storageErr = errors.New("private provider response")
			}
			f.ImportedAssets = importedAssetStub{object: resolved, err: storageErr}
			cmd := CompleteCommand{Principal: upload.Principal, DispatchID: upload.DispatchID, WorkerInstanceID: upload.WorkerInstanceID, Attempt: upload.Attempt, IdempotencyKey: "same-result", Output: map[string]any{"candidates": candidates}}
			receipt, err := f.Complete(context.Background(), cmd)
			if scenario == "valid" || scenario == "legacy_url" {
				if err != nil || receipt.ConductorStatus != "submitted" || len(engine.updateCalls) != 1 {
					t.Fatal("valid media was not acknowledged", err)
				}
				f.ImportedAssets = importedAssetStub{err: errors.New("storage offline after completion")}
				if _, err = f.Complete(context.Background(), cmd); err != nil || len(engine.updateCalls) != 1 {
					t.Fatal("acknowledged replay required storage or resent result", err)
				}
			} else if err == nil || len(engine.updateCalls) != 0 || strings.Contains(err.Error(), "private provider response") {
				t.Fatal("invalid media mutated task or leaked error", err)
			}
		})
	}
}

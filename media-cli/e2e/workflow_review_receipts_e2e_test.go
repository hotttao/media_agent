package e2e

import (
	"encoding/json"
	"os"
	"testing"
)

// Observes a completed sample created by TestWorkflowPersonalWorkerFixture.
// It only replays an existing command; it never submits or starts a new Job.
func TestWorkflowReviewReceiptsGateway(t *testing.T) {
	jobID := os.Getenv("MEDIA_E2E_REVIEW_JOB_ID")
	if jobID == "" {
		t.Skip("set MEDIA_E2E_REVIEW_JOB_ID to a completed fixture sample")
	}
	addr, token := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN")
	if addr == "" || token == "" {
		t.Fatal("gateway and User API Token required")
	}
	status, data := doJSONBearer(t, addr, "GET", "/api/v1/jobs/"+jobID, token, nil)
	var state struct {
		Job struct {
			Status int `json:"status"`
		} `json:"job"`
		Steps []struct {
			ID           string `json:"id"`
			Key          string `json:"step_key"`
			ReviewID     string `json:"last_review_id"`
			ReviewStatus string `json:"last_review_status"`
			Selected     int    `json:"selected_index"`
		} `json:"step_runs"`
	}
	if status != 200 || json.Unmarshal(data, &state) != nil || state.Job.Status != 4 {
		t.Fatal("completed fixture sample required")
	}
	for _, step := range state.Steps {
		if step.Key != "model_image" {
			continue
		}
		if step.ReviewID == "" || step.ReviewStatus != "applied" {
			t.Fatal("accepted decision has not been applied")
		}
		key := "fixture:" + jobID + ":" + step.Key + ":accept"
		status, data = doJSONBearer(t, addr, "POST", "/api/v1/step-runs/"+step.ID+"/confirm", token, map[string]any{"selected_index": step.Selected, "idempotency_key": key})
		var receipt struct {
			ID     string `json:"last_review_id"`
			Status string `json:"last_review_status"`
		}
		if status != 202 || json.Unmarshal(data, &receipt) != nil || receipt.ID != step.ReviewID || receipt.Status != "applied" {
			t.Fatal("terminal replay did not return original receipt")
		}
		status, _ = doJSONBearer(t, addr, "POST", "/api/v1/step-runs/"+step.ID+"/regenerate", token, map[string]any{"idempotency_key": key})
		if status != 409 {
			t.Fatalf("opposite decision status=%d, expected 409", status)
		}
		t.Log("terminal command replay returned original applied receipt; opposite decision rejected")
		return
	}
	t.Fatal("model_image review not found")
}

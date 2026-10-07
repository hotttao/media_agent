package e2e

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This gate consumes exactly one published fixture workflow through a real
// separately running media_worker. It never implements or mocks a worker here.
func TestWorkflowPersonalWorkerFixture(t *testing.T) {
	if os.Getenv("MEDIA_E2E_PERSONAL_WORKER_SAMPLE") != "1" {
		t.Skip("enable the single-run gate with MEDIA_E2E_PERSONAL_WORKER_SAMPLE=1 after starting media_worker")
	}
	addr, token, org := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	if addr == "" || token == "" || org == "" {
		t.Fatal("gateway/User API Token/Organization required")
	}
	wfID := os.Getenv("MEDIA_E2E_SAMPLE_WORKFLOW_ID")
	productID := envOr("MEDIA_E2E_SAMPLE_PRODUCT_ID", "prd_9af6be4e-1b78-4c52-85ec-35fa7234bd50")
	vipID := envOr("MEDIA_E2E_SAMPLE_VIRTUAL_IP_ID", "vip_e3393dcc-faf6-423f-bced-692ac22d0988")
	request := func(method, path string, body any, out any) {
		t.Helper()
		status, data := doJSONBearer(t, addr, method, path, token, body)
		if status < 200 || status >= 300 {
			t.Fatalf("%s %s status=%d", method, path, status)
		}
		if out != nil {
			if err := json.Unmarshal(data, out); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
		}
	}
	// Observe a previously submitted sample without ever submitting another Job.
	if jobID := os.Getenv("MEDIA_E2E_SAMPLE_JOB_ID"); jobID != "" {
		awaitPersonalWorkerFixture(t, request, jobID)
		return
	}
	if wfID == "" {
		t.Fatal("MEDIA_E2E_SAMPLE_WORKFLOW_ID required for a new single sample")
	}
	var user struct {
		ID string `json:"id"`
	}
	request("GET", "/api/auth/me", nil, &user)
	var jobs struct {
		Total int `json:"total"`
		Items []struct {
			Status int    `json:"status"`
			Owner  string `json:"submitted_by"`
		} `json:"items"`
	}
	for page := 1; ; page++ {
		request("GET", fmt.Sprintf("/api/v1/jobs?page_size=200&page=%d", page), nil, &jobs)
		for _, job := range jobs.Items {
			if job.Owner == user.ID && (job.Status == 1 || job.Status == 2 || job.Status == 3) {
				t.Fatal("current User has unfinished jobs; audit before starting the single sample")
			}
		}
		if page*200 >= jobs.Total {
			break
		}
		if len(jobs.Items) == 0 || page >= 1000 {
			t.Fatal("could not audit the full queue before starting the single sample")
		}
	}
	var workflow struct {
		Version int `json:"version"`
		Status  int `json:"status"`
		Steps   []struct {
			Key string `json:"key"`
		} `json:"steps"`
	}
	request("GET", "/api/v1/workflows/"+wfID, nil, &workflow)
	if workflow.Version < 1 || workflow.Status != 2 || len(workflow.Steps) != 4 {
		t.Fatal("expected published four-capability sample")
	}
	query := "?organization_id=" + org
	var product struct {
		ID       string          `json:"id"`
		Name     string          `json:"name"`
		Audience string          `json:"target_audience"`
		Details  string          `json:"product_details"`
		Actions  json.RawMessage `json:"display_actions"`
		Images   []struct {
			Path string `json:"object_path"`
			Main bool   `json:"is_main"`
		} `json:"images"`
	}
	var vip struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Avatar string `json:"avatar_object_path"`
		Body   string `json:"full_body_object_path"`
		Three  string `json:"three_view_object_path"`
		Nine   string `json:"nine_view_object_path"`
	}
	request("GET", "/api/v1/products/"+productID+query, nil, &product)
	request("GET", "/api/v1/virtual-ips/"+vipID+query, nil, &vip)
	resolve := func(path string) string {
		if path == "" {
			return ""
		}
		var result struct {
			URL string `json:"url"`
		}
		// Object keys must be escaped independently of the query structure.
		request("GET", "/api/v1/library-objects:resolve"+query+"&object_path="+url.QueryEscape(path), nil, &result)
		if result.URL == "" {
			t.Fatal("object resolution returned no URL")
		}
		return result.URL
	}
	image := ""
	if len(product.Images) > 0 {
		image = product.Images[0].Path
		for _, item := range product.Images {
			if item.Main {
				image = item.Path
				break
			}
		}
	}
	if image == "" || vip.Body == "" {
		t.Fatal("fixture product/full-body image missing")
	}
	var actions string
	if len(product.Actions) > 0 && string(product.Actions) != "null" {
		if err := json.Unmarshal(product.Actions, &actions); err != nil {
			t.Fatal("product display_actions must be a string")
		}
	}
	input := map[string]any{
		"product":    map[string]any{"id": product.ID, "name": product.Name, "target_audience": product.Audience, "product_details": product.Details, "display_actions": actions, "image_url": resolve(image)},
		"virtual_ip": map[string]any{"id": vip.ID, "name": vip.Name, "avatar_url": resolve(vip.Avatar), "full_body_url": resolve(vip.Body), "three_view_url": resolve(vip.Three), "nine_view_url": resolve(vip.Nine)},
	}
	materials := []map[string]any{}
	for _, kind := range []string{"movement", "pose", "scene"} {
		var page struct {
			Items []struct {
				Path        string `json:"object_path"`
				Content     string `json:"content"`
				Prompt      string `json:"prompt"`
				Description string `json:"description"`
			} `json:"items"`
		}
		request("GET", "/api/v1/materials"+query+"&scope=ai_clothing&asset_kind="+kind+"&page_size=1", nil, &page)
		if len(page.Items) != 1 {
			t.Fatalf("fixture material %s missing", kind)
		}
		item := page.Items[0]
		content := item.Content
		if content == "" {
			content = item.Prompt
		}
		if content == "" {
			content = item.Description
		}
		materials = append(materials, map[string]any{"name": kind, "asset_kind": kind, "url": resolve(item.Path), "content": content})
	}
	input["material"] = materials
	var submitted struct {
		ID string `json:"job_id"`
	}
	workspace := os.Getenv("MEDIA_E2E_WORKSPACE_ID")
	if workspace == "" {
		t.Fatal("target Workspace required for a new sample submission")
	}
	request("POST", "/api/v1/jobs", map[string]any{"workflow_id": wfID, "workflow_version": workflow.Version, "input": input, "organization_id": org, "workspace_id": workspace}, &submitted)
	if submitted.ID == "" {
		t.Fatal("sample submission returned no Job ID")
	}
	t.Logf("single personal-worker sample job_id=%s", submitted.ID)
	awaitPersonalWorkerFixture(t, request, submitted.ID)
	var snapshot struct {
		Job struct {
			Organization string          `json:"organization_id"`
			Workspace    string          `json:"workspace_id"`
			Input        json.RawMessage `json:"input"`
		} `json:"job"`
	}
	request("GET", "/api/v1/jobs/"+submitted.ID, nil, &snapshot)
	encodedInput := string(snapshot.Job.Input)
	if snapshot.Job.Organization != org || snapshot.Job.Workspace != workspace || !strings.Contains(encodedInput, "object_path:materials/"+org+"/") || strings.Contains(strings.ToLower(encodedInput), "x-amz-") || strings.Contains(strings.ToLower(encodedInput), "signature=") {
		t.Fatal("native Job lost its authorized tenant or retained temporary storage credentials")
	}
}

func awaitPersonalWorkerFixture(t *testing.T, request func(string, string, any, any), jobID string) {
	t.Helper()
	var user struct {
		ID string `json:"id"`
	}
	request("GET", "/api/auth/me", nil, &user)
	deadline := time.Now().Add(2 * time.Minute)
	regenerateKey := os.Getenv("MEDIA_E2E_REGENERATE_STEP_KEY")
	retryKey := os.Getenv("MEDIA_E2E_RETRY_STEP_KEY")
	retried := false
	multipleKey := os.Getenv("MEDIA_E2E_MULTIPLE_STEP_KEY")
	selectedIndex := 0
	if raw := os.Getenv("MEDIA_E2E_SELECT_CANDIDATE"); raw != "" {
		var err error
		selectedIndex, err = strconv.Atoi(raw)
		if err != nil || selectedIndex < 0 {
			t.Fatal("invalid selected candidate array position")
		}
	}
	feedback := os.Getenv("MEDIA_E2E_REGENERATE_FEEDBACK")
	if feedback != "" && regenerateKey == "" {
		t.Fatal("feedback requires a regeneration step")
	}
	regenerated, confirmedAfterRegeneration := os.Getenv("MEDIA_E2E_PROVIDER_REGENERATION_ALREADY_REQUESTED") == "1", false
	type reviewReceipt struct {
		ID     string `json:"last_review_id"`
		Status string `json:"last_review_status"`
	}
	for {
		var state struct {
			Job struct {
				Status int    `json:"status"`
				Owner  string `json:"submitted_by"`
			} `json:"job"`
			Steps []struct {
				ID         string `json:"id"`
				Key        string `json:"step_key"`
				Status     int    `json:"status"`
				Asset      string `json:"output_asset_id"`
				Selected   int    `json:"selected_index"`
				Candidates []struct {
					Index int    `json:"index"`
					URL   string `json:"url"`
				} `json:"candidates"`
				Prompt  string `json:"rendered_prompt"`
				Retries int    `json:"retry_count"`
			} `json:"step_runs"`
		}
		request("GET", "/api/v1/jobs/"+jobID, nil, &state)
		if user.ID == "" || state.Job.Owner != user.ID {
			t.Fatal("sample must belong to the authenticated User; refusing observation/approval")
		}
		if state.Job.Status == 5 && retryKey != "" && !retried {
			found := false
			for _, step := range state.Steps {
				if step.Key != retryKey || step.Status != 6 {
					continue
				}
				if regenerateKey == retryKey && !regenerated {
					t.Fatal("fixture failed before the configured regeneration")
				}
				request("POST", "/api/v1/step-runs/"+step.ID+"/retry", map[string]any{"reason": "explicit fixture retry gate"}, nil)
				found, retried = true, true
				t.Logf("retried failed step_key=%s job_id=%s", step.Key, jobID)
			}
			if !found {
				t.Fatal("failed Job did not expose the expected failed StepRun")
			}
			continue
		}
		if state.Job.Status == 5 || state.Job.Status == 6 {
			t.Fatalf("sample terminal failure status=%d", state.Job.Status)
		}
		for _, step := range state.Steps {
			if step.Status == 4 {
				if len(step.Candidates) == 0 {
					t.Fatal("approval has no candidates")
				}
				if step.Key == multipleKey && (len(step.Candidates) != 2 || step.Candidates[0].URL == step.Candidates[1].URL) {
					t.Fatal("expected two distinct mock candidates requiring user approval")
				}
				if step.Key == regenerateKey && !regenerated {
					body := map[string]any{"idempotency_key": "fixture:" + jobID + ":" + step.Key + ":regenerate"}
					if feedback != "" {
						body["feedback"] = feedback
					}
					var receipt, replay reviewReceipt
					request("POST", "/api/v1/step-runs/"+step.ID+"/regenerate", body, &receipt)
					request("POST", "/api/v1/step-runs/"+step.ID+"/regenerate", body, &replay)
					if receipt.ID == "" || replay.ID != receipt.ID {
						t.Fatal("regeneration did not replay durable receipt")
					}
					regenerated = true
					t.Logf("requested one regeneration step_key=%s job_id=%s", step.Key, jobID)
					continue
				}
				index := 0
				if step.Key == multipleKey {
					index = selectedIndex
				}
				if index >= len(step.Candidates) {
					t.Fatal("selected candidate array position out of range")
				}
				body := map[string]any{"selected_index": index, "idempotency_key": "fixture:" + jobID + ":" + step.Key + ":accept"}
				if regenerated && step.Key == regenerateKey && feedback != "" && !strings.HasSuffix(step.Prompt, "\n\n用户重新生成反馈（本轮）：\n"+feedback) {
					// Accepting a decision does not synchronously advance the
					// native loop. Wait for the new rendered generation instead
					// of confirming the still-cached old HUMAN candidates.
					// Missing feedback still fails the bounded final observation.
					continue
				}
				var receipt, replay reviewReceipt
				request("POST", "/api/v1/step-runs/"+step.ID+"/confirm", body, &receipt)
				request("POST", "/api/v1/step-runs/"+step.ID+"/confirm", body, &replay)
				if receipt.ID == "" || replay.ID != receipt.ID {
					t.Fatal("confirmation did not replay durable receipt")
				}
				if regenerated && step.Key == regenerateKey {
					confirmedAfterRegeneration = true
				}
			}
		}
		if state.Job.Status == 4 {
			if retryKey != "" && !retried {
				t.Fatal("sample never exercised the configured failure and retry")
			}
			if regenerateKey != "" && (!regenerated || !confirmedAfterRegeneration) {
				t.Fatal("sample did not regenerate and then accept the configured step")
			}
			if len(state.Steps) != 4 {
				t.Fatal("sample did not finish all four capabilities")
			}
			for _, step := range state.Steps {
				if step.Key == retryKey && step.Retries != 1 {
					t.Fatal("step retry was not recorded exactly once")
				}
				count := 1
				if step.Key == multipleKey {
					count = 2
				}
				if step.Status != 5 || step.Asset == "" || len(step.Candidates) != count {
					t.Fatalf("incomplete fixture step %s", step.Key)
				}
				if step.Key == multipleKey {
					if step.Selected != selectedIndex {
						t.Fatal("multi-candidate selection was not persisted")
					}
					var asset struct {
						Path string `json:"object_path"`
					}
					request("GET", "/api/v1/generated-assets/"+step.Asset+"?organization_id="+os.Getenv("MEDIA_E2E_ORGANIZATION_ID"), nil, &asset)
					selected := step.Candidates[selectedIndex].URL
					path := strings.TrimPrefix(selected, "object_path:")
					if u, err := url.Parse(selected); err == nil && u.Host != "" {
						path = strings.TrimPrefix(u.Path, "/media/")
					}
					if asset.Path == "" || asset.Path != path {
						t.Fatal("final generated asset does not match the user-selected candidate")
					}
				}
			}
			t.Logf("all four steps succeeded and persisted generated assets job_id=%s", jobID)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sample did not converge before deadline job_id=%s", jobID)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

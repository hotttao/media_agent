// workflow_e2e_test.go — media_workflow HTTP e2e 测试。
//
// 前提：media_auth + media_materials + media_workflow server 已起。
//
// 运行：
//
//	MEDIA_AUTH_ADDR=http://127.0.0.1:8001 \
//	MEDIA_MATERIALS_ADDR=http://127.0.0.1:8003 \
//	MEDIA_WORKFLOW_ADDR=http://127.0.0.1:8002 \
//	MEDIA_AUTH_E2E_USER=smoke MEDIA_AUTH_E2E_PASS=smoke123 \
//	go test ./e2e/ -run TestWorkflowE2E -v
package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func skipIfNoWorkflowServer(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("MEDIA_WORKFLOW_ADDR")
	if addr == "" {
		t.Skip("MEDIA_WORKFLOW_ADDR 未设，跳过 media_workflow e2e")
	}
	return addr
}

func TestWorkflowE2E(t *testing.T) {
	authAddr := skipIfNoServer(t)
	matAddr := skipIfNoMaterialsServer(t)
	wfAddr := skipIfNoWorkflowServer(t)
	user := envOr("MEDIA_AUTH_E2E_USER", "smoke")
	pass := envOr("MEDIA_AUTH_E2E_PASS", "smoke123")

	// 1. media_auth login + 创建 service token
	authClient := httpClient(t)
	uid := login(t, authClient, authAddr, user, pass)

	status, body := doJSON(t, authClient, "POST", authAddr+"/api/auth/tokens", map[string]string{"name": "e2e_workflow"})
	if status != 201 {
		t.Fatalf("create service token: status=%d body=%s", status, body)
	}
	var tokResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &tokResp); err != nil || tokResp.Token == "" {
		t.Fatalf("token unmarshal: err=%v body=%s", err, body)
	}

	// HTTP client with Bearer
	wfClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	matClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	_ = matClient
	_ = matAddr

	// 2. 创建 template（scope=ai_clothing，output_asset_kind=first_frame）
	status, body = doJSON(t, wfClient, "POST", wfAddr+"/api/v1/templates?scope=ai_clothing&created_by="+uid, map[string]any{
		"name":               "e2e_template",
		"prompt_body":        "以 {scene.url} 为场景生成首帧图",
		"output_asset_kind":  "first_frame",
		"platform":           "gpt",
		"gen_way":            "i2i",
		"needs_confirm":      true,
		"candidate_count":    2,
		"inputs": []map[string]any{
			{"name": "scene", "resource_type": "MATERIAL", "required": true},
		},
	})
	if status != 200 {
		t.Fatalf("create template: status=%d body=%s", status, body)
	}
	var tpl struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(body, &tpl); err != nil || tpl.ID == "" {
		t.Fatalf("create template unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created template id=%s version=%d", tpl.ID, tpl.Version)

	// 3. 创建 workflow（单步 DAG）
	status, body = doJSON(t, wfClient, "POST", wfAddr+"/api/v1/workflows?scope=ai_clothing&created_by="+uid, map[string]any{
		"name":             "e2e_workflow",
		"output_step_key":  "gen",
		"steps": []map[string]any{
			{
				"key":              "gen",
				"step_order":       1,
				"template_id":      tpl.ID,
				"template_version": tpl.Version,
				"slot_bindings": map[string]any{
					"scene": map[string]any{"source": "WORKFLOW_INPUT"},
				},
			},
		},
	})
	if status != 200 {
		t.Fatalf("create workflow: status=%d body=%s", status, body)
	}
	var wf struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(body, &wf); err != nil || wf.ID == "" {
		t.Fatalf("create workflow unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created workflow id=%s version=%d", wf.ID, wf.Version)

	// 4. validate workflow（不保存）
	status, body = doJSON(t, wfClient, "POST", wfAddr+"/api/v1/workflows/validate", map[string]any{
		"scope":            "ai_clothing",
		"output_step_key":  "gen",
		"steps": []map[string]any{
			{
				"key":              "gen",
				"step_order":       1,
				"template_id":      tpl.ID,
				"template_version": tpl.Version,
				"slot_bindings": map[string]any{
					"scene": map[string]any{"source": "WORKFLOW_INPUT"},
				},
			},
		},
	})
	if status != 200 {
		t.Fatalf("validate workflow: status=%d body=%s", status, body)
	}
	var valResult struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(body, &valResult); err != nil {
		t.Fatalf("validate unmarshal: %v", err)
	}
	if !valResult.Valid {
		t.Fatalf("validate workflow: not valid, body=%s", body)
	}

	// 5. submit job（带自包含 bundle）
	status, body = doJSON(t, wfClient, "POST", wfAddr+"/api/v1/jobs?workflow_id="+wf.ID+"&submitted_by="+uid, map[string]any{
		"workflow_version": wf.Version,
		"input": map[string]any{
			"material": []map[string]any{
				{"name": "scene", "url": "https://example.com/scene.png"},
			},
		},
	})
	if status != 200 {
		t.Fatalf("submit job: status=%d body=%s", status, body)
	}
	var job struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &job); err != nil || job.JobID == "" {
		t.Fatalf("submit job unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("submitted job id=%s status=%s", job.JobID, job.Status)

	// 6. get job（应含 step_runs）
	status, body = doJSON(t, wfClient, "GET", wfAddr+"/api/v1/jobs/"+job.JobID, nil)
	if status != 200 {
		t.Fatalf("get job: status=%d body=%s", status, body)
	}
	var jobDetail struct {
		Job struct {
			Status string `json:"status"`
		} `json:"job"`
		StepRuns []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			StepKey string `json:"step_key"`
		} `json:"step_runs"`
	}
	if err := json.Unmarshal(body, &jobDetail); err != nil {
		t.Fatalf("get job unmarshal: %v", err)
	}
	if len(jobDetail.StepRuns) == 0 {
		t.Fatal("get job: no step_runs")
	}
	// root step 应为 pending
	sr := jobDetail.StepRuns[0]
	if sr.Status != "pending" && sr.Status != "claimed" {
		t.Errorf("step_run status=%s, want pending or claimed", sr.Status)
	}
	t.Logf("step_run id=%s key=%s status=%s", sr.ID, sr.StepKey, sr.Status)

	// 7. claim step_run（模拟 worker）
	status, body = doJSON(t, wfClient, "POST", wfAddr+"/api/v1/step-runs/claim", map[string]any{
		"platform":  "gpt",
		"gen_way":   "i2i",
		"worker_id": "e2e_worker",
	})
	if status != 200 {
		t.Fatalf("claim step_run: status=%d body=%s", status, body)
	}
	var claim struct {
		StepRunID      string `json:"step_run_id"`
		RenderedPrompt string `json:"rendered_prompt"`
		AssetKind      string `json:"asset_kind"`
		CandidateCount int    `json:"candidate_count"`
	}
	if err := json.Unmarshal(body, &claim); err != nil || claim.StepRunID == "" {
		t.Fatalf("claim unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("claimed step_run id=%s prompt=%q asset_kind=%s", claim.StepRunID, claim.RenderedPrompt, claim.AssetKind)

	// 8. complete step_run（2 候选，needs_confirm=true → NEED_CONFIRM）
	status, body = doJSON(t, wfClient, "POST", wfAddr+"/api/v1/step-runs/"+claim.StepRunID+"/complete", map[string]any{
		"candidates": []map[string]any{
			{"index": 1, "url": "https://example.com/out1.png", "is_primary": true},
			{"index": 2, "url": "https://example.com/out2.png"},
		},
	})
	if status != 200 {
		t.Fatalf("complete step_run: status=%d body=%s", status, body)
	}
	var completed struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &completed); err != nil {
		t.Fatalf("complete unmarshal: %v", err)
	}
	if completed.Status != "need_confirm" {
		t.Errorf("complete: status=%s, want need_confirm", completed.Status)
	}
	t.Logf("completed → status=%s", completed.Status)

	// 9. confirm step_run（选第 1 个）
	status, body = doJSON(t, wfClient, "POST", wfAddr+"/api/v1/step-runs/"+claim.StepRunID+"/confirm", map[string]any{
		"selected_index": 1,
	})
	if status != 200 {
		t.Fatalf("confirm step_run: status=%d body=%s", status, body)
	}
	var confirmed struct {
		Status        string `json:"status"`
		OutputAssetID string `json:"output_asset_id"`
	}
	if err := json.Unmarshal(body, &confirmed); err != nil {
		t.Fatalf("confirm unmarshal: %v", err)
	}
	if confirmed.Status != "succeeded" {
		t.Errorf("confirm: status=%s, want succeeded", confirmed.Status)
	}
	if confirmed.OutputAssetID == "" {
		t.Error("confirm: empty output_asset_id")
	}
	t.Logf("confirmed → status=%s output_asset_id=%s", confirmed.Status, confirmed.OutputAssetID)

	// 10. get job → 应 succeeded（单步 DAG，sink succeeded → job succeeded）
	status, body = doJSON(t, wfClient, "GET", wfAddr+"/api/v1/jobs/"+job.JobID, nil)
	if status != 200 {
		t.Fatalf("get job final: status=%d", status)
	}
	if err := json.Unmarshal(body, &jobDetail); err != nil {
		t.Fatalf("get job final unmarshal: %v", err)
	}
	if jobDetail.Job.Status != "succeeded" {
		t.Errorf("job final status=%s, want succeeded", jobDetail.Job.Status)
	}
	t.Logf("job final status=%s", jobDetail.Job.Status)

	// 11. cleanup: delete workflow + template
	doJSON(t, wfClient, "DELETE", wfAddr+"/api/v1/workflows/"+wf.ID, nil)
	doJSON(t, wfClient, "DELETE", wfAddr+"/api/v1/templates/"+tpl.ID, nil)

	// revoke token
	doJSON(t, authClient, "DELETE", authAddr+"/api/auth/tokens/"+tokResp.Token, nil)

	t.Log("✅ media_workflow e2e passed")
}

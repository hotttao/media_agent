// workflow_full_e2e_test.go - 核心链路 e2e:素材 -> 模板 -> workflow -> SubmitJob -> claim -> complete -> 验产物。
//
// 覆盖最复杂的业务链路(见梳理):
//  1. login + service token
//  2. create material + virtual_ip(media_materials)
//  3. create template + publish(media_workflow)
//  4. create workflow + publish
//  5. submit job(caller 组装 bundle)
//  6. claim step run(worker pull)
//  7. complete step run(候选 -> finalize -> 产物落 generated_assets)
//  8. 验 job SUCCEEDED + generated_asset 存在
//
// 符合 e2e.md 规范:media-cli/e2e + 裸 HTTP + Bearer;服务由 docker-compose 起(环境变量指定 addr,未设 skip)。
//
// 运行:
//
//	MEDIA_AUTH_ADDR=http://127.0.0.1:8001 \
//	MEDIA_MATERIALS_ADDR=http://127.0.0.1:8003 \
//	MEDIA_WORKFLOW_ADDR=http://127.0.0.1:8002 \
//	MEDIA_AUTH_E2E_USER=smoke MEDIA_AUTH_E2E_PASS=smoke123 \
//	go test ./e2e/ -run TestWorkflowFull -v
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"testing"
)

// TestWorkflowFull 核心链路:素材 -> 模板 -> workflow -> submit -> claim -> complete -> 验产物。
func TestWorkflowFull(t *testing.T) {
	authAddr := envOr("MEDIA_AUTH_ADDR", "")
	matAddr := envOr("MEDIA_MATERIALS_ADDR", "")
	wfAddr := envOr("MEDIA_WORKFLOW_ADDR", "")
	if authAddr == "" || matAddr == "" || wfAddr == "" {
		t.Skip("MEDIA_AUTH_ADDR / MEDIA_MATERIALS_ADDR / MEDIA_WORKFLOW_ADDR 未设,跳过(需先起服务)")
	}
	user := envOr("MEDIA_AUTH_E2E_USER", "smoke")
	pass := envOr("MEDIA_AUTH_E2E_PASS", "smoke123")
	scope := "ai_clothing"
	ts := time.Now().Format("0102150405")

	// 1. login + service token
	uid, token := materialsBearerToken(t, authAddr, user, pass)
	t.Logf("login uid=%s token=%s...", uid, token[:12])

	// 2. create material(scene 图)+ virtual_ip
	matStatus, matBody := doJSONBearer(t, matAddr, "POST", "/api/v1/materials?scope="+scope+"&owner_user_id="+uid, token, map[string]any{
		"name": "e2e_scene_"+ts, "asset_kind": "scene", "url": "https://mock/scene.jpg", "visibility": "public",
	})
	if matStatus != http.StatusOK {
		t.Fatalf("create material: status=%d body=%s", matStatus, matBody)
	}
	var mat struct {
		ID string `json:"id"`
	}
	json.Unmarshal(matBody, &mat)
	t.Logf("created material id=%s", mat.ID)
	defer doJSONBearer(t, matAddr, "DELETE", "/api/v1/materials/"+mat.ID, token, nil)

	vipStatus, vipBody := doJSONBearer(t, matAddr, "POST", "/api/v1/virtual-ips?scope="+scope+"&owner_user_id="+uid, token, map[string]any{
		"name": "e2e_vip_"+ts, "avatar_url": "https://mock/avatar.jpg", "visibility": "public",
	})
	if vipStatus != http.StatusOK {
		t.Fatalf("create virtual_ip: status=%d body=%s", vipStatus, vipBody)
	}
	var vip struct {
		ID string `json:"id"`
	}
	json.Unmarshal(vipBody, &vip)
	t.Logf("created vip id=%s", vip.ID)
	defer doJSONBearer(t, matAddr, "DELETE", "/api/v1/virtual-ips/"+vip.ID, token, nil)

	// 3. create template(Go template prompt_body + needs_confirm=false 单候选)
	tplStatus, tplBody := doJSONBearer(t, wfAddr, "POST", "/api/v1/templates?scope="+scope+"&created_by="+uid, token, map[string]any{
		"name": "e2e_template_"+ts, "prompt_body": "生成 {{.scene.url}} 的图",
		"inputs": []map[string]any{
			{"name": "scene", "resource_type": "MATERIAL", "required": true, "display_order": 1},
		},
		"output_asset_kind": "model_image", "platform": "gpt", "gen_way": "i2i",
		"needs_confirm": false, "candidate_count": 1,
	})
	if tplStatus != http.StatusOK {
		t.Fatalf("create template: status=%d body=%s", tplStatus, tplBody)
	}
	var tpl struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	json.Unmarshal(tplBody, &tpl)
	t.Logf("created template id=%s version=%d", tpl.ID, tpl.Version)
	defer doJSONBearer(t, wfAddr, "DELETE", "/api/v1/templates/"+tpl.ID, token, nil)

	// publish template
	pubStatus, _ := doJSONBearer(t, wfAddr, "POST", "/api/v1/templates/"+tpl.ID+"/publish", token, nil)
	if pubStatus != http.StatusOK {
		t.Fatalf("publish template: status=%d", pubStatus)
	}
	t.Logf("published template")

	// 4. create workflow(单 step gen,引用 template,binding scene = WORKFLOW_INPUT)
	wfStatus, wfBody := doJSONBearer(t, wfAddr, "POST", "/api/v1/workflows?scope="+scope+"&created_by="+uid, token, map[string]any{
		"name": "e2e_workflow_"+ts, "output_step_key": "gen",
		"steps": []map[string]any{
			{
				"key": "gen", "name": "gen", "step_order": 1,
				"template_id": tpl.ID, "template_version": tpl.Version,
				"input_bindings": map[string]any{
					"scene": map[string]any{"source": 1}, // WORKFLOW_INPUT=1
				},
			},
		},
	})
	if wfStatus != http.StatusOK {
		t.Fatalf("create workflow: status=%d body=%s", wfStatus, wfBody)
	}
	var wf struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	json.Unmarshal(wfBody, &wf)
	t.Logf("created workflow id=%s version=%d", wf.ID, wf.Version)
	defer doJSONBearer(t, wfAddr, "DELETE", "/api/v1/workflows/"+wf.ID, token, nil)

	// publish workflow
	wfPubStatus, _ := doJSONBearer(t, wfAddr, "POST", "/api/v1/workflows/"+wf.ID+"/publish", token, nil)
	if wfPubStatus != http.StatusOK {
		t.Fatalf("publish workflow: status=%d", wfPubStatus)
	}
	t.Logf("published workflow")

	// 5. submit job(caller 组装 bundle:material scene 的 url)
	jobStatus, jobBody := doJSONBearer(t, wfAddr, "POST", "/api/v1/jobs?workflow_id="+wf.ID+"&submitted_by="+uid, token, map[string]any{
		"workflow_version": wf.Version,
		"input": map[string]any{
			"material": []map[string]any{
				{"name": "scene", "url": "https://mock/scene.jpg", "content": ""},
			},
		},
	})
	if jobStatus != http.StatusOK {
		t.Fatalf("submit job: status=%d body=%s", jobStatus, jobBody)
	}
	var job struct {
		JobID  string `json:"job_id"`
		Status int `json:"status"`
	}
	json.Unmarshal(jobBody, &job)
	t.Logf("submitted job id=%s status=%d", job.JobID, job.Status)

	// 6. claim step run(模拟 worker)
	claimStatus, claimBody := doJSONBearer(t, wfAddr, "POST", "/api/v1/step-runs/claim", token, map[string]any{
		"platform": "gpt", "gen_way": "i2i", "gen_type": "", "worker_id": "e2e_worker",
	})
	if claimStatus != http.StatusOK {
		t.Fatalf("claim: status=%d body=%s", claimStatus, claimBody)
	}
	var claim struct {
		StepRunID      string `json:"step_run_id"`
		RenderedPrompt string `json:"rendered_prompt"`
	}
	json.Unmarshal(claimBody, &claim)
	t.Logf("claimed step_run id=%s prompt=%q", claim.StepRunID, claim.RenderedPrompt)

	// 验 prompt 渲染(Go template {{.scene.url}} 被替换)
	if claim.RenderedPrompt != "生成 https://mock/scene.jpg 的图" {
		t.Errorf("prompt render mismatch: got %q, want %q", claim.RenderedPrompt, "生成 https://mock/scene.jpg 的图")
	}

	// 7. complete step run(单候选,needs_confirm=false -> 直接 SUCCEEDED + finalize)
	compStatus, _ := doJSONBearer(t, wfAddr, "POST", "/api/v1/step-runs/"+claim.StepRunID+"/complete", token, map[string]any{
		"candidates": []map[string]any{
			{"index": 1, "url": "https://mock/generated.jpg", "content": "", "is_primary": true},
		},
	})
	if compStatus != http.StatusOK {
		t.Fatalf("complete: status=%d", compStatus)
	}
	t.Logf("completed step_run")

	// 8. 验 job SUCCEEDED + generated_asset 存在
	getStatus, getBody := doJSONBearer(t, wfAddr, "GET", "/api/v1/jobs/"+job.JobID, token, nil)
	if getStatus != http.StatusOK {
		t.Fatalf("get job: status=%d", getStatus)
	}
	var jobResult struct {
		Job struct {
			Status int `json:"status"`
		} `json:"job"`
		StepRuns []struct {
			Status        int    `json:"status"`
			OutputAssetID string `json:"output_asset_id"`
		} `json:"step_runs"`
	}
	json.Unmarshal(getBody, &jobResult)
	t.Logf("job raw=%s", string(getBody))
	t.Logf("job status=%d step_runs=%d", jobResult.Job.Status, len(jobResult.StepRuns))

	if jobResult.Job.Status != 4 { // JobStatus SUCCEEDED=4
		t.Errorf("job status: got %d, want 4(succeeded)", jobResult.Job.Status)
	}
	if len(jobResult.StepRuns) == 0 || jobResult.StepRuns[0].Status != 5 { // StepRunStatus SUCCEEDED=5
		t.Errorf("step_run[0] status: got %d, want 5(succeeded)", jobResult.StepRuns[0].Status)
	}
	if len(jobResult.StepRuns) == 0 || jobResult.StepRuns[0].OutputAssetID == "" {
		t.Errorf("step_run[0].output_asset_id empty (finalizeStepRun should have set it)")
	} else {
		t.Logf("generated_asset id=%s", jobResult.StepRuns[0].OutputAssetID)
		// 验 generated_asset 落库(GET /api/v1/generated-assets/:id)
		genStatus, _ := doJSONBearer(t, matAddr, "GET", "/api/v1/generated-assets/"+jobResult.StepRuns[0].OutputAssetID, token, nil)
		if genStatus != http.StatusOK {
			t.Errorf("get generated_asset: status=%d", genStatus)
		} else {
			t.Logf("verified generated_asset in media_materials")
		}
	}

	fmt.Println("=== TestWorkflowFull PASS: 素材->模板->workflow->submit->claim->complete->产物 全链路通过 ===")
}

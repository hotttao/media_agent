// workflow_need_confirm_e2e_test.go - need_confirm + media_gen worker claim + ConfirmStepRun。
//
// 与 TestWorkflowFull 区别：
//   - template needs_confirm=true, candidate_count=4
//   - 用 media_gen worker（StartWorker）自动 claim + complete，不手动调 ClaimStepRun
//   - worker complete 后 step_run -> need_confirm（4 候选）
//   - e2e 调 ConfirmStepRun 选定候选 -> SUCCEEDED
//
// 运行：
//
//	MEDIA_AUTH_ADDR=... MEDIA_MATERIALS_ADDR=... MEDIA_WORKFLOW_ADDR=... MEDIA_GEN_ADDR=... \
//	MEDIA_AUTH_E2E_USER=e2e-smoke MEDIA_AUTH_E2E_PASS=smoke123 \
//	go test ./e2e/ -run TestWorkflowNeedConfirm -v
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestWorkflowNeedConfirm(t *testing.T) {
	authAddr := envOr("MEDIA_AUTH_ADDR", "")
	matAddr := envOr("MEDIA_MATERIALS_ADDR", "")
	wfAddr := envOr("MEDIA_WORKFLOW_ADDR", "")
	genAddr := envOr("MEDIA_GEN_ADDR", "")
	if authAddr == "" || matAddr == "" || wfAddr == "" || genAddr == "" {
		t.Skip("需 MEDIA_AUTH/MATERIALS/WORKFLOW/GEN_ADDR 全设")
	}
	user := envOr("MEDIA_AUTH_E2E_USER", "e2e-smoke")
	pass := envOr("MEDIA_AUTH_E2E_PASS", "smoke123")
	scope := "ai_clothing"
	ts := time.Now().Format("0102150405")

	uid, token := materialsBearerToken(t, authAddr, user, pass)
	t.Logf("login uid=%s", uid)

	// 1. create material + vip
	_, matBody := doJSONBearer(t, matAddr, "POST", "/api/v1/materials?scope="+scope+"&owner_user_id="+uid, token, map[string]any{
		"name": "nc_scene_" + ts, "asset_kind": "scene", "url": "https://mock/nc_scene.jpg", "visibility": "public",
	})
	var mat struct {
		ID string `json:"id"`
	}
	json.Unmarshal(matBody, &mat)

	_, vipBody := doJSONBearer(t, matAddr, "POST", "/api/v1/virtual-ips?scope="+scope+"&owner_user_id="+uid, token, map[string]any{
		"name": "nc_vip_" + ts, "avatar_url": "https://mock/nc_avatar.jpg", "visibility": "public",
	})
	var vip struct {
		ID string `json:"id"`
	}
	json.Unmarshal(vipBody, &vip)
	t.Logf("material=%s vip=%s", mat.ID, vip.ID)

	// 2. create template (needs_confirm=true, candidate_count=4)
	_, tplBody := doJSONBearer(t, wfAddr, "POST", "/api/v1/templates?scope="+scope+"&created_by="+uid, token, map[string]any{
		"name": "nc_tpl_" + ts, "prompt_body": "生成 {{.scene.url}} 的图",
		"inputs": []map[string]any{
			{"name": "scene", "resource_type": "MATERIAL", "required": true, "display_order": 1},
		},
		"output_asset_kind": "model_image", "platform": "gpt", "gen_way": "i2i",
		"needs_confirm": true, "candidate_count": 4,
	})
	var tpl struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	json.Unmarshal(tplBody, &tpl)
	t.Logf("template id=%s v%d", tpl.ID, tpl.Version)
	doJSONBearer(t, wfAddr, "POST", "/api/v1/templates/"+tpl.ID+"/publish", token, nil)

	// 3. create workflow + publish
	_, wfBody := doJSONBearer(t, wfAddr, "POST", "/api/v1/workflows?scope="+scope+"&created_by="+uid, token, map[string]any{
		"name": "nc_wf_" + ts, "output_step_key": "gen",
		"steps": []map[string]any{
			{"key": "gen", "name": "gen", "step_order": 1,
				"template_id": tpl.ID, "template_version": tpl.Version,
				"input_bindings": map[string]any{"scene": map[string]any{"source": 1}}},
		},
	})
	var wf struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	json.Unmarshal(wfBody, &wf)
	t.Logf("workflow id=%s v%d", wf.ID, wf.Version)
	doJSONBearer(t, wfAddr, "POST", "/api/v1/workflows/"+wf.ID+"/publish", token, nil)

	// 4. submit job
	_, jobBody := doJSONBearer(t, wfAddr, "POST", "/api/v1/jobs?workflow_id="+wf.ID+"&submitted_by="+uid, token, map[string]any{
		"workflow_version": wf.Version,
		"input": map[string]any{
			"material": []map[string]any{{"name": "scene", "url": "https://mock/nc_scene.jpg"}},
		},
	})
	var job struct {
		JobID string `json:"job_id"`
	}
	json.Unmarshal(jobBody, &job)
	t.Logf("job id=%s", job.JobID)

	// 5. StartWorker（media_gen worker 自动 claim + complete）
	_, startBody := doJSONBearer(t, genAddr, "POST", "/api/v1/workers:start", token, map[string]any{
		"worker_id": "e2e_nc_worker_" + ts, "platform": "gpt", "gen_way": "i2i",
		"gen_type": "", "poll_interval_ms": 1000,
	})
	t.Logf("worker start: %s", string(startBody))

	// 6. Poll GetJob until step_run need_confirm（worker claim + complete -> need_confirm）
	var stepRunID string
	var foundNeedConfirm bool
	for i := 0; i < 15; i++ {
		time.Sleep(2 * time.Second)
		_, getBody := doJSONBearer(t, wfAddr, "GET", "/api/v1/jobs/"+job.JobID, token, nil)
		var jr struct {
			StepRuns []struct {
				ID     string `json:"id"`
				Status int    `json:"status"`
			} `json:"step_runs"`
		}
		json.Unmarshal(getBody, &jr)
		if len(jr.StepRuns) > 0 {
			stepRunID = jr.StepRuns[0].ID
			if jr.StepRuns[0].Status == 4 { // NEED_CONFIRM=4 (proto SR_NEED_CONFIRM)
				foundNeedConfirm = true
				t.Logf("step_run %s -> NEED_CONFIRM (poll %d)", stepRunID, i)
				break
			}
			t.Logf("poll %d: step_run status=%d", i, jr.StepRuns[0].Status)
		}
	}
	if !foundNeedConfirm {
		t.Fatalf("step_run did not reach NEED_CONFIRM in time")
	}

	// 7. ConfirmStepRun（选定候选 index=0）
	_, _ = doJSONBearer(t, wfAddr, "POST", "/api/v1/step-runs/"+stepRunID+"/confirm", token, map[string]any{
		"selected_index": 0,
	})
	t.Logf("confirmed step_run index=0")

	// 8. Poll until job SUCCEEDED
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		_, getBody := doJSONBearer(t, wfAddr, "GET", "/api/v1/jobs/"+job.JobID, token, nil)
		var jr struct {
			Job struct {
				Status int `json:"status"`
			} `json:"job"`
			StepRuns []struct {
				Status        int    `json:"status"`
				OutputAssetID string `json:"output_asset_id"`
			} `json:"step_runs"`
		}
		json.Unmarshal(getBody, &jr)
		if jr.Job.Status == 4 { // SUCCEEDED=4
			t.Logf("job SUCCEEDED (poll %d)", i)
			if len(jr.StepRuns) > 0 && jr.StepRuns[0].OutputAssetID != "" {
				// 验 generated_asset
				genStatus, _ := doJSONBearer(t, matAddr, "GET", "/api/v1/generated-assets/"+jr.StepRuns[0].OutputAssetID, token, nil)
				if genStatus == http.StatusOK {
					t.Logf("generated_asset verified: %s", jr.StepRuns[0].OutputAssetID)
				}
			}
			fmt.Println("=== TestWorkflowNeedConfirm PASS: media_gen worker claim + need_confirm + confirm ===")
			return
		}
		t.Logf("poll %d: job status=%d", i, jr.Job.Status)
	}
	t.Fatalf("job did not reach SUCCEEDED in time")
}

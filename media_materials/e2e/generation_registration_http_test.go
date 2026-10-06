//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"media_agent/media_materials/biz/dal"
	repo "media_agent/media_materials/biz/dal/repo/generated_asset"
	handler "media_agent/media_materials/biz/handler/generated_asset"
	router "media_agent/media_materials/biz/router/generated_asset"
	service "media_agent/media_materials/biz/service/generated_asset"
)

// Real IDL binding, generated router, internal authorization, handler, service
// and PostgreSQL. ut executes in-process: dropping a response here is not a
// real TCP outage or an OS process restart. Storage and sources are controlled.
func TestGenerationRegistrationHTTPReplayPersistence(t *testing.T) {
	db := resultDatabase(t)
	previous := handler.Svc
	t.Cleanup(func() { handler.Svc = previous })
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "isolated-test-internal-token")
	storage := &registrationStorage{}
	rewire := func() {
		r := repo.NewRepo(db)
		s := service.NewService(r, registrationProduct{}, nil)
		s.ConfigureResultReceipts(r, dal.NewTxRunner(db))
		s.ConfigureHistoryImport(storage, registrationKinds{})
		s.ConfigureNativeOutputStorage("https://storage.example", "media")
		handler.Svc = s
	}
	rewire()
	h := server.Default()
	router.Register(h)
	cmd := map[string]any{"scope": "ai_clothing", "asset_kind": "first_frame", "name": "frame", "owner_user_id": "user",
		"url":    "https://storage.example/media/materials/org/ai_clothing/material/frame.png?signature=temporary",
		"job_id": "job", "step_run_id": "step", "result_key": strings.Repeat("a", 64), "template_id": "tpl", "template_version": 1, "source_product_id": "product"}
	request := func(authorized bool) (int, []byte) {
		body, err := json.Marshal(cmd)
		if err != nil {
			t.Fatal("request encoding failed")
		}
		headers := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
		if authorized {
			headers = append(headers, ut.Header{Key: "X-Media-Materials-Internal-Token", Value: "isolated-test-internal-token"})
		}
		r := ut.PerformRequest(h.Engine, "POST", "/api/internal/v1/generated-assets:upsert", &ut.Body{Body: strings.NewReader(string(body)), Len: len(body)}, headers...).Result()
		return r.StatusCode(), append([]byte(nil), r.Body()...)
	}
	count := func(table string, want int64) {
		t.Helper()
		var n int64
		if db.Table("media_materials."+table).Count(&n).Error != nil || n != want {
			t.Fatal("unexpected persisted count")
		}
	}
	if status, _ := request(false); status != 401 {
		t.Fatal("internal write authorization missing")
	}
	count("generated_assets", 0)
	// The calling side deliberately discards the successful response. Read only
	// the durable receipt as the acceptance oracle, not the response's asset ID.
	if status, _ := request(true); status != 200 {
		t.Fatal("first registration failed")
	}
	var original string
	if db.Table("media_materials.generated_asset_results").Select("asset_id").Scan(&original).Error != nil || original == "" {
		t.Fatal("accepted receipt absent")
	}
	storage.unavailable.Store(true)
	calls := storage.calls.Load()
	rewire()
	cmd["url"] = "https://storage.example/media/materials/org/ai_clothing/material/frame.png?signature=expired"
	status, body := request(true)
	var replay struct {
		ID string `json:"id"`
	}
	if status != 200 || json.Unmarshal(body, &replay) != nil || replay.ID != original || storage.calls.Load() != calls {
		t.Fatal("response replay did not return original receipt")
	}
	count("generated_assets", 1)
	count("generated_asset_results", 1)
	cmd["content"] = "changed payload"
	if status, _ := request(true); status != 409 {
		t.Fatal("changed same-key payload did not conflict")
	}
	delete(cmd, "content")
	cmd["scope"] = "foreign"
	if status, _ := request(true); status != 400 {
		t.Fatal("receipt bypassed source scope verification")
	}
	cmd["scope"] = "ai_clothing"
	storage.unavailable.Store(false)
	cmd["result_key"] = strings.Repeat("b", 64)
	status, body = request(true)
	var next struct {
		ID string `json:"id"`
	}
	if status != 200 || json.Unmarshal(body, &next) != nil || next.ID == "" || next.ID == original {
		t.Fatal("new round did not create independent HTTP result")
	}
	count("generated_assets", 2)
	count("generated_asset_results", 2)
}

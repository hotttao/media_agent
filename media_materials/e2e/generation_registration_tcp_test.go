//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"media_agent/media_materials/biz/dal"
	repo "media_agent/media_materials/biz/dal/repo/generated_asset"
	handler "media_agent/media_materials/biz/handler/generated_asset"
	router "media_agent/media_materials/biz/router/generated_asset"
	service "media_agent/media_materials/biz/service/generated_asset"
)

// Controlled TCP fault adapter forwards to the actual Hertz route in-process.
// The client/server socket is real; the deployed service, gateway, Storage and
// Workflow client are not under test. No business service is started here.
func TestGenerationRegistrationTCPLostResponse(t *testing.T) {
	db := resultDatabase(t)
	previous := handler.Svc
	t.Cleanup(func() { handler.Svc = previous })
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "isolated-tcp-token")
	r := repo.NewRepo(db)
	s := service.NewService(r, registrationProduct{}, nil)
	storage := &registrationStorage{}
	s.ConfigureResultReceipts(r, dal.NewTxRunner(db))
	s.ConfigureHistoryImport(storage, registrationKinds{})
	s.ConfigureNativeOutputStorage("https://storage.example", "media")
	handler.Svc = s
	h := server.Default()
	router.Register(h)
	var requests atomic.Int64
	committed := make(chan bool, 1)
	fault := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(io.LimitReader(req.Body, 16385))
		if err != nil || len(body) > 16384 {
			w.WriteHeader(400)
			return
		}
		response := ut.PerformRequest(h.Engine, req.Method, req.URL.Path, &ut.Body{Body: bytes.NewReader(body), Len: len(body)},
			ut.Header{Key: "Content-Type", Value: "application/json"},
			ut.Header{Key: "X-Media-Materials-Internal-Token", Value: req.Header.Get("X-Media-Materials-Internal-Token")}).Result()
		if requests.Add(1) == 1 {
			// Only close after the real handler has finished its committed write.
			connection, _, err := w.(http.Hijacker).Hijack()
			committed <- response.StatusCode() == 200 && err == nil
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode())
		_, _ = w.Write(response.Body())
	}))
	defer fault.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	cmd := map[string]any{"scope": "ai_clothing", "asset_kind": "first_frame", "name": "frame", "owner_user_id": "user",
		"url":    "https://storage.example/media/materials/org/ai_clothing/material/frame.png?signature=first",
		"job_id": "job", "step_run_id": "step", "result_key": strings.Repeat("a", 64), "template_id": "tpl", "template_version": 1, "source_product_id": "product"}
	request := func() (*http.Response, error) {
		body, _ := json.Marshal(cmd)
		req, _ := http.NewRequest("POST", fault.URL+"/api/internal/v1/generated-assets:upsert", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Media-Materials-Internal-Token", "isolated-tcp-token")
		return client.Do(req)
	}
	response, err := request()
	if response != nil {
		response.Body.Close()
	}
	if err == nil {
		t.Fatal("caller unexpectedly received first committed response")
	}
	select {
	case ok := <-committed:
		if !ok {
			t.Fatal("fault occurred before successful registration")
		}
	case <-time.After(time.Second):
		t.Fatal("committed boundary was not observed")
	}
	if requests.Load() != 1 {
		t.Fatal("transport silently retried ambiguous registration")
	}
	var original string
	if db.Table("media_materials.generated_asset_results").Select("asset_id").Scan(&original).Error != nil || original == "" {
		t.Fatal("accepted receipt absent after TCP loss")
	}
	storage.unavailable.Store(true)
	calls := storage.calls.Load()
	cmd["url"] = "https://storage.example/media/materials/org/ai_clothing/material/frame.png?signature=expired"
	response, err = request()
	if err != nil {
		t.Fatal("explicit same-key replay failed")
	}
	defer response.Body.Close()
	var replay struct {
		ID string `json:"id"`
	}
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&replay) != nil || replay.ID != original || storage.calls.Load() != calls {
		t.Fatal("TCP replay did not return original durable result")
	}
	for _, table := range []string{"generated_assets", "generated_asset_results"} {
		var count int64
		if db.Table("media_materials."+table).Count(&count).Error != nil || count != 1 {
			t.Fatal("TCP replay duplicated or lost resource")
		}
	}
}

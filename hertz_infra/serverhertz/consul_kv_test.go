//go:build e2e

// consul_kv_test.go：Consul KV 配置中心拉取的 e2e 集成测试（需真实 consul agent）。
//
// 验证 problem.md「测试从配置中心拉取配置」端到端：
//   - newConsulKVSource.Get() 从 Consul KV 读 JSON 配置；
//   - NewServerSuite 订阅 + loader.Watch 的 blocking-query 在 KV 变更后触发 reload，
//     限流规则从配置中心拉取并热替换。
//
// 运行前提：docker-compose 起的 consul agent（默认 127.0.0.1:8500），或通过 CONSUL_ADDR 指定。
//   go test -tags=e2e ./serverhertz/ -run TestConsulKVRateLimitPull -v
// 无 consul 时自动 skip。
package serverhertz

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/alibaba/sentinel-golang/core/flow"
	consulapi "github.com/hashicorp/consul/api"

	config "media_agent/hertz_infra/config"
)

func consulAddr() string {
	if a := os.Getenv("CONSUL_ADDR"); a != "" {
		return a
	}
	return "127.0.0.1:8500"
}

// skipIfNoConsul 探活 consul agent，不可达则 skip。
func skipIfNoConsul(t *testing.T) *consulapi.Client {
	t.Helper()
	cc, err := consulapi.NewClient(&consulapi.Config{Address: consulAddr()})
	if err != nil {
		t.Skipf("consul client: %v", err)
	}
	if _, err := cc.Agent().Self(); err != nil {
		t.Skipf("consul agent unreachable at %s: %v", consulAddr(), err)
	}
	return cc
}

// TestConsulKVRateLimitPull：KV 推送 v1(threshold=50) → 起 server suite 订阅 →
// KV 推送 v2(threshold=10, warmup) → 断言 sentinel 规则被热替换。
func TestConsulKVRateLimitPull(t *testing.T) {
	t.Cleanup(func() { _ = flow.ClearRules() })
	cc := skipIfNoConsul(t)

	const dataID = "test/serverhertz/consul_kv_ratelimit"
	// 清理历史 KV。
	_, _ = cc.KV().Delete(dataID, nil)
	t.Cleanup(func() { _, _ = cc.KV().Delete(dataID, nil) })

	putKV := func(threshold float64, strategy string) {
		t.Helper()
		v, _ := json.Marshal(map[string]any{
			"rate_limit": []any{
				map[string]any{
					"enabled":                   true,
					"resource":                  "test:GET:/r",
					"threshold":                 threshold,
					"stat_interval_ms":          1000,
					"token_calculate_strategy": strategy,
					"control_behavior":         "reject",
				},
			},
		})
		if _, err := cc.KV().Put(&consulapi.KVPair{Key: dataID, Value: v}, nil); err != nil {
			t.Fatalf("kv put: %v", err)
		}
	}

	// 预置 v1。
	putKV(50, "direct")

	base := writeTempYAML(t, `
app:
  name: test-server
  version: v1
  env: test
consul:
  enabled: false
consul_kv:
  enabled: true
  address: `+consulAddr()+`
  data_id: `+dataID+`
rate_limit:
  - enabled: true
    resource: "test:GET:/r"
    threshold: 1
    stat_interval_ms: 1000
`)
	loader, err := config.NewLoaderFromPath(base)
	if err != nil {
		t.Fatalf("NewLoaderFromPath: %v", err)
	}
	t.Cleanup(loader.Close)

	suite := NewServerSuite(loader) // 订阅 loadFlowRules
	if err := loader.Attach(suite.KVSource()); err != nil {
		t.Fatalf("Attach KV source: %v", err)
	}
	if err := loader.Watch(); err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Attach 后 KV v1 覆盖文件基线 → threshold 50。
	if r := firstFlowRule(t); r.Threshold != 50 {
		t.Fatalf("after attach: threshold want 50, got %v", r.Threshold)
	}

	// 推送 v2：threshold 10 + warmup。blocking-query 应 promptly 触发 reload。
	putKV(10, "warmup")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if r := flow.GetRules(); len(r) == 1 && r[0].Threshold == 10 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	r := firstFlowRule(t)
	if r.Threshold != 10 {
		t.Errorf("threshold: want 10, got %v", r.Threshold)
	}
	if r.TokenCalculateStrategy != flow.WarmUp {
		t.Errorf("TokenCalculateStrategy: want WarmUp, got %v", r.TokenCalculateStrategy)
	}
}

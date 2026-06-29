// circuitbreaker_test.go：熔断规则映射 + NewAppClientSuite 订阅配置中心（fake Source）热更新单测。
//
// 验证 problem.md「测试从配置中心拉取配置」：NewAppClientSuite 内 loader.Subscribe 在 fake Source
// 推送新 circuit_breaker 配置后，刷新 atomic 规则 + 重载 sentinel circuitbreaker 规则，
// 且新增的 max_allowed_rt_ms（slow_request_ratio 必需）正确映射。
//
// 注意：sentinel 状态进程级全局，cleanup 调 circuitbreaker.ClearRules()；不可 -parallel。
package clienthertz

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alibaba/sentinel-golang/core/circuitbreaker"

	config "media_agent/hertz_infra/config"
)

type fakeSource struct {
	mu       sync.Mutex
	data     map[string]any
	priority int
	onChange func()
}

func (f *fakeSource) Get() (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data == nil {
		return nil, nil
	}
	out := make(map[string]any, len(f.data))
	for k, v := range f.data {
		out[k] = v
	}
	return out, nil
}
func (f *fakeSource) Watch(onChange func()) error { f.onChange = onChange; return nil }
func (f *fakeSource) Close() error                 { return nil }
func (f *fakeSource) Priority() int                { return f.priority }
func (f *fakeSource) Set(data map[string]any) {
	f.mu.Lock()
	f.data = data
	f.mu.Unlock()
	if f.onChange != nil {
		f.onChange()
	}
}

// TestNewAppClientSuiteCircuitBreakerHotReload 验证 NewAppClientSuite 订阅热更新：
// 初始 error_ratio → fake 推送 slow_request_ratio(含 max_allowed_rt_ms) → 规则替换。
func TestNewAppClientSuiteCircuitBreakerHotReload(t *testing.T) {
	t.Cleanup(func() { _ = circuitbreaker.ClearRules() })

	base := writeTempYAML(t, `
app:
  name: test-client
  version: v1
  env: test
client:
  media_example:
    enabled: true
    service_name: media-example
    base_domain: http://127.0.0.1:8888
    use_discovery: false
    timeout:
      enabled: true
      read_timeout_ms: 5000
    circuit_breaker:
      enabled: true
      strategy: error_ratio
      threshold: 0.5
      stat_interval_ms: 10000
      retry_timeout_ms: 5000
      min_request_amount: 5
`)
	loader, err := config.NewLoaderFromPath(base)
	if err != nil {
		t.Fatalf("NewLoaderFromPath: %v", err)
	}
	t.Cleanup(loader.Close)

	appSuite, err := NewClientSuite(loader).NewAppClientSuite("media_example")
	if err != nil {
		t.Fatalf("NewAppClientSuite: %v", err)
	}

	// 初始：error_ratio。
	if cb := appSuite.CircuitBreaker(); cb.GetStrategy() != "error_ratio" {
		t.Fatalf("initial strategy: want error_ratio, got %q", cb.GetStrategy())
	}
	if r := firstCBRule(t); r.Strategy != circuitbreaker.ErrorRatio {
		t.Fatalf("initial sentinel strategy: want ErrorRatio, got %v", r.Strategy)
	}

	// fake 推送 v2：slow_request_ratio + max_allowed_rt_ms。
	fake := &fakeSource{priority: 20, data: map[string]any{
		"client": map[string]any{
			"media_example": map[string]any{
				"circuit_breaker": map[string]any{
					"enabled":            true,
					"strategy":           "slow_request_ratio",
					"threshold":          0.5,
					"stat_interval_ms":   10000,
					"retry_timeout_ms":   5000,
					"min_request_amount": 5,
					"max_allowed_rt_ms":  1000,
				},
			},
		},
	}}
	if err := loader.Attach(fake); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	cb := appSuite.CircuitBreaker()
	if cb.GetStrategy() != "slow_request_ratio" {
		t.Errorf("strategy: want slow_request_ratio, got %q", cb.GetStrategy())
	}
	if cb.GetMaxAllowedRtMs() != 1000 {
		t.Errorf("MaxAllowedRtMs: want 1000, got %d", cb.GetMaxAllowedRtMs())
	}
	r := firstCBRule(t)
	if r.Strategy != circuitbreaker.SlowRequestRatio {
		t.Errorf("sentinel strategy: want SlowRequestRatio, got %v", r.Strategy)
	}
	if r.MaxAllowedRtMs != 1000 {
		t.Errorf("sentinel MaxAllowedRtMs: want 1000, got %d", r.MaxAllowedRtMs)
	}
	if r.Resource != "test-client->media_example" {
		t.Errorf("Resource: want test-client->media_example, got %q", r.Resource)
	}
}

func firstCBRule(t *testing.T) circuitbreaker.Rule {
	t.Helper()
	rules := circuitbreaker.GetRules()
	if len(rules) != 1 {
		t.Fatalf("expect 1 cb rule, got %d: %+v", len(rules), rules)
	}
	return rules[0]
}

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "conf.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	return p
}

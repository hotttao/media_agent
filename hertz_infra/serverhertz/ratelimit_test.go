// ratelimit_test.go：限流规则映射 + 配置中心（fake Source）热更新单测。
//
// 验证 problem.md「测试从配置中心拉取配置」：通过 fake config.Source 模拟 Consul KV
// 推送，断言 NewServerSuite 订阅的 loadFlowRules 收到新配置后 sentinel flow 规则被替换，
// 且新增的 token_calculate_strategy / control_behavior / warmup / throttling 字段正确映射。
//
// 注意：sentinel 状态进程级全局，flow.ClearRules() 在 cleanup 调用；本测试不可 -parallel。
package serverhertz

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alibaba/sentinel-golang/core/flow"

	configpb "media_agent/hertz_gen/config"
	config "media_agent/hertz_infra/config"
)

// fakeSource 是 config.Source 的测试实现，模拟 Consul KV 推送。
// Set 更新数据并触发 onChange（若已 Watch），驱动 Loader reload。
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

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "conf.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	return p
}

// TestLoadFlowRulesMapping 直接验证新字段映射：warmup/throttling 及其参数。
func TestLoadFlowRulesMapping(t *testing.T) {
	t.Cleanup(func() { _ = flow.ClearRules() })

	rules := []*configpb.RateLimitConfig{
		{
			Enabled:                true,
			Resource:               "test:GET:/r",
			Threshold:              10,
			StatIntervalMs:         1000,
			TokenCalculateStrategy: "warmup",
			ControlBehavior:        "throttling",
			MaxQueueingTimeMs:      500,
			WarmUpPeriodSec:        10,
			WarmUpColdFactor:       3,
		},
	}
	if err := loadFlowRules(rules); err != nil {
		t.Fatalf("loadFlowRules: %v", err)
	}
	got := flow.GetRules()
	if len(got) != 1 {
		t.Fatalf("expect 1 rule, got %d", len(got))
	}
	r := got[0]
	if r.TokenCalculateStrategy != flow.WarmUp {
		t.Errorf("TokenCalculateStrategy: want WarmUp, got %v", r.TokenCalculateStrategy)
	}
	if r.ControlBehavior != flow.Throttling {
		t.Errorf("ControlBehavior: want Throttling, got %v", r.ControlBehavior)
	}
	if r.MaxQueueingTimeMs != 500 {
		t.Errorf("MaxQueueingTimeMs: want 500, got %d", r.MaxQueueingTimeMs)
	}
	if r.WarmUpPeriodSec != 10 {
		t.Errorf("WarmUpPeriodSec: want 10, got %d", r.WarmUpPeriodSec)
	}
	if r.WarmUpColdFactor != 3 {
		t.Errorf("WarmUpColdFactor: want 3, got %d", r.WarmUpColdFactor)
	}
}

// TestServerSuiteRateLimitHotReload 验证 NewServerSuite 订阅 + fake Source 热更新：
// 初始 direct/reject(50) → fake 推送 warmup/throttling(10) → sentinel 规则被替换。
func TestServerSuiteRateLimitHotReload(t *testing.T) {
	t.Cleanup(func() { _ = flow.ClearRules() })

	base := writeTempYAML(t, `
app:
  name: test-server
  version: v1
  env: test
rate_limit:
  - enabled: true
    resource: "test:GET:/r"
    threshold: 50
    stat_interval_ms: 1000
    token_calculate_strategy: direct
    control_behavior: reject
`)
	loader, err := config.NewLoaderFromPath(base)
	if err != nil {
		t.Fatalf("NewLoaderFromPath: %v", err)
	}
	t.Cleanup(loader.Close)

	NewServerSuite(loader) // 内部 loader.Subscribe(loadFlowRules)；副作用即订阅

	// 初始规则：direct/reject, threshold 50。
	if r := firstFlowRule(t); r.Threshold != 50 || r.TokenCalculateStrategy != flow.Direct ||
		r.ControlBehavior != flow.Reject {
		t.Fatalf("initial rule mismatch: %+v", r)
	}

	// fake Source 模拟配置中心推送 v2（warmup/throttling, threshold 10）。
	fake := &fakeSource{priority: 20, data: map[string]any{
		"rate_limit": []any{
			map[string]any{
				"enabled":                   true,
				"resource":                  "test:GET:/r",
				"threshold":                 10,
				"stat_interval_ms":          1000,
				"token_calculate_strategy": "warmup",
				"control_behavior":         "throttling",
				"max_queueing_time_ms":     500,
				"warm_up_period_sec":       10,
				"warm_up_cold_factor":      3,
			},
		},
	}}
	if err := loader.Attach(fake); err != nil { // Attach 立即触发 reload
		t.Fatalf("Attach: %v", err)
	}

	r := firstFlowRule(t)
	if r.Threshold != 10 {
		t.Errorf("Threshold: want 10, got %v", r.Threshold)
	}
	if r.TokenCalculateStrategy != flow.WarmUp {
		t.Errorf("TokenCalculateStrategy: want WarmUp, got %v", r.TokenCalculateStrategy)
	}
	if r.ControlBehavior != flow.Throttling {
		t.Errorf("ControlBehavior: want Throttling, got %v", r.ControlBehavior)
	}
	if r.MaxQueueingTimeMs != 500 {
		t.Errorf("MaxQueueingTimeMs: want 500, got %d", r.MaxQueueingTimeMs)
	}
}

func firstFlowRule(t *testing.T) flow.Rule {
	t.Helper()
	rules := flow.GetRules()
	if len(rules) != 1 {
		t.Fatalf("expect 1 flow rule, got %d: %+v", len(rules), rules)
	}
	return rules[0]
}

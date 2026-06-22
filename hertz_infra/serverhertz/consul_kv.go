// serverhertz/consul_kv.go：Consul KV 配置源。
//
// 实现 config.Source 接口，对接 Consul KV blocking-query。
// 根据 cfg.GetConsulKv().GetEnabled() 决定是否构造——
// 不启用时 ServerSuite.KVSource() 返回 nil，Loader.Attach(nil) 无操作。
//
// 与 serverhertz/registry.go（consul 服务注册）是同一 pattern：
//   - 都消费 cfg 里一个 consul_* 顶级块
//   - 都根据 enabled 字段决定是否挂载
//   - Consul SDK 只出现在 serverhertz，不泄漏到业务层 / config 包
package serverhertz

import (
	"encoding/json"
	"log"
	"time"

	consulapi "github.com/hashicorp/consul/api"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/config"
)

// consulKVSource 实现 config.Source，对接 Consul KV。
//
// - Get() 拉取当前 KV value（JSON）→ unmarshal 成 map[string]any
// - Watch(onChange) 启动 blocking-query，KV 变更时回调 onChange
// - Close() 停止 blocking-query goroutine
// - Priority() = 20（覆盖文件源的 10）
type consulKVSource struct {
	client  *consulapi.Client
	key     string
	stopCh  chan struct{}
	stopped bool
}

// newConsulKVSource 根据 ConsulKVConfig 构造 Source。
// 调用方应在 cfg.GetConsulKv().GetEnabled() 为 true 时才调本函数。
func newConsulKVSource(cfg *configpb.ConsulKVConfig) (*consulKVSource, error) {
	client, err := consulapi.NewClient(&consulapi.Config{Address: cfg.GetAddress()})
	if err != nil {
		return nil, err
	}
	return &consulKVSource{
		client: client,
		key:    cfg.GetDataId(),
		stopCh: make(chan struct{}),
	}, nil
}

// Get 拉取当前 KV value（JSON）并 unmarshal 成 map。
// KV 不存在返回 nil, nil（不视为错误，Loader 跳过此 source）。
func (s *consulKVSource) Get() (map[string]any, error) {
	kv, _, err := s.client.KV().Get(s.key, nil)
	if err != nil {
		return nil, err
	}
	if kv == nil || len(kv.Value) == 0 {
		return nil, nil
	}
	tree := map[string]any{}
	if err := json.Unmarshal(kv.Value, &tree); err != nil {
		return nil, err
	}
	return tree, nil
}

// Watch 启动 Consul KV blocking-query，变更时调 onChange。
//
// blocking-query 通过 WaitIndex 实现：服务端阻塞直到有新版本或超时（30s），
// LastIndex 严格大于上一次才说明有变更。
func (s *consulKVSource) Watch(onChange func()) error {
	go s.loop(onChange)
	return nil
}

func (s *consulKVSource) loop(onChange func()) {
	var lastIndex uint64
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}
		_, meta, err := s.client.KV().Get(s.key, &consulapi.QueryOptions{
			WaitIndex: lastIndex,
			WaitTime:  30 * time.Second,
		})
		if err != nil {
			log.Printf("consul_kv: watch error: %v", err)
			select {
			case <-s.stopCh:
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if meta.LastIndex <= lastIndex {
			continue
		}
		lastIndex = meta.LastIndex
		onChange()
	}
}

func (s *consulKVSource) Close() error {
	if !s.stopped {
		s.stopped = true
		close(s.stopCh)
	}
	return nil
}

// Priority = 20，覆盖文件源（10）。
func (s *consulKVSource) Priority() int { return 20 }

// 编译期断言：consulKVSource 实现 config.Source。
var _ config.Source = (*consulKVSource)(nil)

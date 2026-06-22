package config

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
)

// Loader 管理多个 Source，统一 reload + 通知订阅者。
//
// 使用流程：
//
//	loader, _ := config.NewLoader()                  // 内部创建 file source + 首次 reload
//	cfg := loader.Current()
//	suite := serverhertz.NewServerSuite(cfg)
//	loader.Attach(suite.KVSource())                  // 按需 attach KV source（nil 时无操作）
//	loader.Watch()                                   // 统一启动所有 source 的 watch
//	defer loader.Close()                             // 退出时释放所有 source
//
// config 包只依赖 Source 接口，不依赖任何具体源的 SDK。
type Loader struct {
	sources []Source

	mu      sync.RWMutex
	current *Config
	subs    []func(*Config)

	watched bool
}

// NewLoader 创建 Loader，内部自动创建 file source 并 attach。
// 不启动 watch——watch 由 Watch() 显式启动。
//
// 首次加载失败直接返回 error，调用方应 fatal 终止启动。
func NewLoader() (*Loader, error) {
	if err := godotenvLoad(); err != nil {
		log.Printf("config: .env not loaded: %v", err)
	}
	return NewLoaderFromPath(resolveConfigPath())
}

// NewLoaderFromPath 显式指定 yaml 路径创建 Loader，不读 .env、不看 APP_ENV。
func NewLoaderFromPath(path string) (*Loader, error) {
	l := &Loader{}
	fs, err := newFileSource(path)
	if err != nil {
		return nil, fmt.Errorf("config: create file source: %w", err)
	}
	if err := l.attach(fs); err != nil {
		return nil, err
	}
	return l, nil
}

// Attach 添加一个 Source。
// 传 nil = 无操作（调用方可以无条件调 Attach(suite.KVSource())，enabled=false 时 suite 返回 nil）。
// 添加后立即触发一次 reload（合并新 source 的当前值）。
func (l *Loader) Attach(src Source) error {
	if src == nil {
		return nil
	}
	return l.attach(src)
}

func (l *Loader) attach(src Source) error {
	l.mu.Lock()
	l.sources = append(l.sources, src)
	l.mu.Unlock()
	return l.reload()
}

// Watch 启动所有已 attach 的 source 的 Watch。
// 任一 source 变更都触发 reload（重新 Get 所有 source 并合并）。
// 只能调一次，重复调返回 nil。
func (l *Loader) Watch() error {
	l.mu.Lock()
	if l.watched {
		l.mu.Unlock()
		return nil
	}
	l.watched = true
	sources := append([]Source{}, l.sources...)
	l.mu.Unlock()

	for _, src := range sources {
		src := src
		if err := src.Watch(func() {
			if err := l.reload(); err != nil {
				log.Printf("config: reload failed: %v", err)
			}
		}); err != nil {
			return fmt.Errorf("config: start source watch: %w", err)
		}
	}
	return nil
}

// Current 返回当前 *Config 快照（只读，调用方不应修改）。
func (l *Loader) Current() *Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.current
}

// Subscribe 注册一个变更回调。
// 回调在 reload 完成后被调用，接收最新的 *Config 快照。
// 回调禁止阻塞 IO；否则会拖慢所有 watcher。
func (l *Loader) Subscribe(handler func(*Config)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.subs = append(l.subs, handler)
}

// reload 重新从所有 source 拉取值并合并。
//
// 合并规则：按 Priority 升序，后面的覆盖前面的。
// 文件源 (priority=10) 先读作基线，KV 源 (priority=20) 后读覆盖。
// 任一 source 失败保留旧值并记录日志，不阻断其他 source。
func (l *Loader) reload() error {
	l.mu.RLock()
	sources := append([]Source{}, l.sources...)
	l.mu.RUnlock()

	// 按 priority 升序排列
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].Priority() < sources[j].Priority()
	})

	base := map[string]any{}
	for _, src := range sources {
		data, err := src.Get()
		if err != nil {
			log.Printf("config: source get (skip): %v", err)
			continue
		}
		if data == nil {
			continue
		}
		base = deepMerge(base, data)
	}

	// map → JSON → protojson → *Config
	jsonBytes, err := json.Marshal(base)
	if err != nil {
		return fmt.Errorf("config: json marshal: %w", err)
	}
	cfg := &Config{}
	opts := protojsonUnmarshalOptions()
	if err := opts.Unmarshal(jsonBytes, cfg); err != nil {
		return fmt.Errorf("config: protojson unmarshal: %w", err)
	}

	l.mu.Lock()
	l.current = cfg
	subs := append([]func(*Config){}, l.subs...)
	l.mu.Unlock()

	for _, sub := range subs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("config: subscriber panic: %v", r)
				}
			}()
			sub(cfg)
		}()
	}
	return nil
}

// Close 停止所有 source。main 退出时 defer 调用。
func (l *Loader) Close() {
	l.mu.Lock()
	sources := l.sources
	l.sources = nil
	l.mu.Unlock()
	for _, src := range sources {
		_ = src.Close()
	}
}

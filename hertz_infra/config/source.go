// Package config 的 Source 接口是配置源的统一抽象。
//
// 所有源（文件 / Consul KV / 未来 Nacos 等）都实现此接口。
// config 包只依赖这一个接口，不依赖任何具体源的 SDK。
//
// 调用方（如 serverhertz）构造具体 Source 实现并通过 Loader.Attach 注入。
package config

// Source 是配置源的统一抽象。
//
// Get 返回已解析的配置树（map[string]any），源自己负责 yaml/json unmarshal。
// Loader 只做 deep-merge，不关心源的数据格式。
type Source interface {
	// Get 当前值。
	// 文件源返回 yaml 解析后的 map；KV 源返回 json 解析后的 map。
	// 无数据返回 nil, nil（不视为错误）。
	Get() (map[string]any, error)

	// Watch 订阅源变更。变更时调 onChange（无参，Loader 自己会重新 Get 所有 source）。
	// 实现方负责启动自己的监听机制（fsnotify / blocking-query）。
	Watch(onChange func()) error

	// Close 释放监听资源。
	Close() error

	// Priority 优先级。数值大的覆盖数值小的。
	// 文件源 = 10（基线）；KV 源 = 20（覆盖基线）。
	Priority() int
}

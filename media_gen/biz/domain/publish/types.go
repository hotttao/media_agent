// Package publish 定义把视频 generated_asset 发布到目标平台的领域模型。
//
// 本期 mock：不调 media_materials / media_crawler，不持久化。
// 真实实现需新增 publishes 表 + 异步轮询平台状态，到时再建表。
package publish

// Target -- 发布目标平台。与 media_crawler 平台概念对齐。
type Target string

const (
	TargetXHS    Target = "xhs"
	TargetDouyin Target = "douyin"
	TargetBili   Target = "bilibili"
)

// SupportedTargets 当前支持的发布目标（静态列表）。
// 真实实现可由配置驱动。
var SupportedTargets = []Target{TargetXHS, TargetDouyin, TargetBili}

// State -- 发布状态机。
type State string

const (
	StatePending    State = "PENDING"
	StatePublishing State = "PUBLISHING"
	StateSucceeded  State = "SUCCEEDED"
	StateFailed     State = "FAILED"
)

// Command -- PublishVideo 入参。
type Command struct {
	AssetID  string // 已生成的 generated_asset id（视频）
	Target   Target
	Metadata map[string]string // 平台特定元数据（标题/标签等），可选
}

// Result -- PublishVideo 出参。
type Result struct {
	AssetID           string
	Target            Target
	State             State
	PublishURL        string // 平台上的发布 url（mock）
	PlatformPublishID string // 平台返回的发布 id（mock），供轮询用
	PublishedAt       int64  // unix ms
	FailureReason     string
}

package workflow

import "context"

// TemplateSnapshot workflow 闸1 校验期需要的 template 版本快照（原语类型）。
type TemplateSnapshot struct {
	ID                string
	Scope             string
	Status            string
	PromptBody        string
	OutputAssetKind   string
	Platform          string
	GenWay            string
	GenType           string
	NeedsConfirm      bool
	CandidateCount    int
	CapabilityKey     string
	CapabilityVersion int
	InvocationMapping map[string]InvocationExpressionSnapshot
	Inputs            []SlotSnapshot
}

// SlotSnapshot template input slot 快照。
type SlotSnapshot struct {
	Name         string
	AssetKind    string
	ResourceType string
	Required     bool
}

type InvocationExpressionSnapshot struct {
	Op    string
	Slot  string
	Items []InvocationExpressionSnapshot
}

// TemplateLookup template 版本查询端口（由 template repo/service 实现，main.go 注入）。
// workflow 闸1 用它加载 step 引用的 (template_id, version) 快照，校验存在/未 archived/scope 一致，
// 并取 slots 做 binding 校验 + input_schema 派生。
type TemplateLookup interface {
	GetVersion(ctx context.Context, id string, version int) (*TemplateSnapshot, error)
}

// SlotSnapshot → domain.Slot 的桥接（供 input_schema 派生用）。
type Slot struct {
	Name         string
	ResourceType string
	Required     bool
}

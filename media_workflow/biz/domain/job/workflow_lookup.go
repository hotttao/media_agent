package job

import (
	"context"

	wfdomain "media_agent/media_workflow/biz/domain/workflow"
)

// WorkflowLookup workflow 查询端口（由 wfrepo.Repo 实现，main.go 注入）。
// job service 用它加载 workflow（steps + bindings + input_schema）。
type WorkflowLookup interface {
	Get(ctx context.Context, id string) (*wfdomain.Workflow, error)
}

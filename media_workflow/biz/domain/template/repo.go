package template

import "context"

// Repo template domain 仓库接口。
type Repo interface {
	// Save 创建 template（含 inputs）。version=1。
	Save(ctx context.Context, t *Template) error
	// Get 取当前版本（含 inputs）。
	Get(ctx context.Context, id string) (*Template, error)
	// GetVersion 取指定版本快照（从 template_versions）。
	GetVersion(ctx context.Context, id string, version int) (*TemplateVersion, error)
	// List 分页过滤。
	List(ctx context.Context, q ListQuery) ([]*Template, int, error)
	// Update 整体覆盖（含 inputs）；version+1；旧版本入 template_versions。
	Update(ctx context.Context, t *Template, snapshot *TemplateVersion) error
	// UpdateStatus 改 status（archive/publish）。
	UpdateStatus(ctx context.Context, id string, status Status) error
	// Delete 软删。
	Delete(ctx context.Context, id string) error
}

// GrantRepo template_grants 仓库。
type GrantRepo interface {
	AddGrant(ctx context.Context, g *Grant) error
	RemoveGrant(ctx context.Context, templateID, teamID string) error
	ListGrants(ctx context.Context, templateID string) ([]*Grant, error)
}

type ListQuery struct {
	Scope      string
	Name       string
	Visibility string
	Limit      int
	Offset     int
}

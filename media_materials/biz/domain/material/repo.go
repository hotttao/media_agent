package material

import "context"

// Repo material domain 仓库接口（由 dal/repo 实现）。
type Repo interface {
	Save(ctx context.Context, m *Material) error
	Get(ctx context.Context, id string) (*Material, error)
	List(ctx context.Context, q ListQuery) ([]*Material, int, error)
	Update(ctx context.Context, m *Material) error
	Delete(ctx context.Context, id string) error
}

// BindingRepo material_bindings 仓库接口。
type BindingRepo interface {
	AddBinding(ctx context.Context, b *Binding) error
	RemoveBinding(ctx context.Context, materialID string, ownerKind BindingOwnerKind, ownerID string) error
	ListByOwner(ctx context.Context, ownerKind BindingOwnerKind, ownerID string) ([]*Material, error)
	// EndBindings 失效 owner 的全部生效 binding（置 bindings_end_at = now），owner 软删时调用。
	EndBindings(ctx context.Context, ownerKind, ownerID string) error
}

// ListQuery 列表过滤。
type ListQuery struct {
	OrganizationID string
	Scope          string
	AssetKinds     []string
	Owner          string
	Visibility     string
	CallerID       string
	CallerTeamIDs  []string
	Limit          int
	Offset         int
}

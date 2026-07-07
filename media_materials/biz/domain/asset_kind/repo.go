package asset_kind

import "context"

// Repo asset_kind domain 的仓库接口（由 dal/repo 实现）。
// repo 从 context 取连接（dal.FromContext），支持上层事务透传。
type Repo interface {
	// Save 创建 asset_kind。name 冲突 → 返回错误。
	Save(ctx context.Context, ak *AssetKind) error
	// Get 按 name 取。
	Get(ctx context.Context, name string) (*AssetKind, error)
	// List 按 resource_type / media_type / status 过滤分页。
	List(ctx context.Context, q ListQuery) ([]*AssetKind, int, error)
	// Update 改 description / status（锁列不可改，service 层保证）。
	Update(ctx context.Context, ak *AssetKind) error
	// Delete 删除（仅 ENABLED 可删；SYSTEM 由 service 拦）。
	Delete(ctx context.Context, name string) error
	// BatchGet 批量取（给 media_workflow 校验期用）；未命中的不返回，调用方比对 not_found。
	BatchGet(ctx context.Context, names []string) ([]*AssetKind, error)
	// CountByAssetKind 查引用了某 asset_kind 的 material 数量（in-use 校验用）。
	CountMaterialsByAssetKind(ctx context.Context, name string) (int64, error)
	// CountGeneratedAssetsByAssetKind 查引用了某 asset_kind 的 generated_asset 数量。
	CountGeneratedAssetsByAssetKind(ctx context.Context, name string) (int64, error)
}

// ListQuery 列表过滤。
type ListQuery struct {
	ResourceType string
	MediaType    string
	Status       string
	Limit        int
	Offset       int
}

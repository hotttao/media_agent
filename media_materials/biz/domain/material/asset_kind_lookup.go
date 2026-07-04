package material

import "context"

// AssetKindLookup asset_kind 查询端口（由 asset_kind service 实现，main.go 注入）。
// material service 用它校验 asset_kind 已注册、resource_type=MATERIAL、未停用。
// 返回原语类型，避免跨 domain 包 import。
type AssetKindLookup interface {
	Lookup(ctx context.Context, name string) (resourceType, mediaType, status string, err error)
}

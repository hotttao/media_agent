package material

import "context"

// AssetKindLookup asset_kind 查询端口（由 asset_kind service 实现，main.go 注入）。
type AssetKindLookup interface {
	Lookup(ctx context.Context, name string) (resourceType, mediaType, status string, err error)
}

// OwnerScopeLookup 取 virtual_ip / product 的 scope（同 scope 校验用）。
// 由 virtual_ip / product service 实现，main.go 注入。
type OwnerScopeLookup interface {
	GetOwnerScope(ctx context.Context, ownerKind, ownerID string) (string, error)
}

package shared

import "context"

// AssetKindInfo asset_kind 查询结果（原语类型，跨 service 共享）。
type AssetKindInfo struct {
	Name         string
	ResourceType string
	MediaType    string
	Status       string
}

// AssetKindLookup asset_kind 查询端口（由 materials client 实现，main.go 注入到 template/workflow service）。
type AssetKindLookup interface {
	BatchGet(ctx context.Context, names []string) (map[string]*AssetKindInfo, []string, error)
}

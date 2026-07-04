package asset_kind

import "media_agent/media_materials/biz/domain/asset_kind"

// RegisterCommand 注册新 asset_kind。
type RegisterCommand struct {
	Name         string
	ResourceType string
	MediaType    string
	SourceTable  string
	Description  string
}

// UpdateCommand 改 description / status（锁列不可改，service 校验）。
type UpdateCommand struct {
	Name        string
	Description string
	Status      string // 可空 = 不改
}

// ListQuery 列表过滤（透传 domain.ListQuery）。
type ListQuery = asset_kind.ListQuery

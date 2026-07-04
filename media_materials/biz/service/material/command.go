package material

import domain "media_agent/media_materials/biz/domain/material"

// CreateCommand 创建 material。
type CreateCommand struct {
	Scope       string
	Name        string
	AssetKind   string
	URL         string
	Content     string
	OwnerUserID string
	Visibility  string
	TeamID      string
}

// UpdateCommand 改 name/url/content/visibility/team_id（scope/asset_kind 不可改）。
type UpdateCommand struct {
	ID         string
	Name       string
	URL        string
	Content    string
	Visibility string
	TeamID     string
	CallerID   string // 写操作校验 owner
}

// AddBindingCommand material ↔ owner。
type AddBindingCommand struct {
	MaterialID string
	OwnerKind  string // virtual_ip / product
	OwnerID    string
	BoundBy    string
}

type ListQuery = domain.ListQuery

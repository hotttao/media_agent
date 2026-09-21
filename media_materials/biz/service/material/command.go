package material

import domain "media_agent/media_materials/biz/domain/material"

// CreateCommand 创建 material。
type CreateCommand struct {
	OrganizationID string
	Scope          string
	Name           string
	Description    string
	Prompt         string
	AssetKind      string
	ObjectPath     string
	Content        string
	Metadata       string
	OwnerUserID    string
	Visibility     string
	TeamID         string
}

// UpdateCommand 改 name/url/content/visibility/team_id（scope/asset_kind 不可改）。
type UpdateCommand struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
	Prompt         string
	ObjectPath     string
	Content        string
	Metadata       string
	Visibility     string
	TeamID         string
	CallerID       string // 写操作校验 owner
}

// AddBindingCommand material ↔ owner。
type AddBindingCommand struct {
	MaterialID string
	OwnerKind  string // virtual_ip / product
	OwnerID    string
	BoundBy    string
}

type ListQuery = domain.ListQuery

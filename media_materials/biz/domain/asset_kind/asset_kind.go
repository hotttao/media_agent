package asset_kind

import "time"

// ResourceType 资源物理类别（决定落哪张表）。
type ResourceType string

const (
	ResourceTypeMaterial  ResourceType = "MATERIAL"
	ResourceTypeAsset     ResourceType = "ASSET"
	ResourceTypeProduct   ResourceType = "PRODUCT"
	ResourceTypeVirtualIP ResourceType = "VIRTUAL_IP"
	ResourceTypeText      ResourceType = "TEXT"
)

// MediaType 媒体形态。
type MediaType string

const (
	MediaTypeImage MediaType = "IMAGE"
	MediaTypeText  MediaType = "TEXT"
	MediaTypeVideo MediaType = "VIDEO"
)

// Status asset_kind 状态。
// SYSTEM：系统预置，不可删，name/resource_type/media_type/source_table 锁列不可改。
// ENABLED：自定义，正常 CRUD。
// DISABLED：软停用，列表仍展示但 template 创建/校验拒绝引用。
type Status string

const (
	StatusSystem   Status = "SYSTEM"
	StatusEnabled  Status = "ENABLED"
	StatusDisabled Status = "DISABLED"
)

func (s Status) Usable() bool { return s == StatusSystem || s == StatusEnabled }

// AssetKind 资源类别注册表实体（全局，无 scope）。
// name / resource_type / media_type / source_table 创建后锁定不可改。
type AssetKind struct {
	Name         string
	ResourceType ResourceType
	MediaType    MediaType
	SourceTable  string
	Description  string
	Status       Status
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

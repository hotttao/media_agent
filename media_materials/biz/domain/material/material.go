package material

import "time"

// Visibility 资源可见性（资源级 ACL）。
// private=owner；team=owner+team 成员（team 求值依赖 media_auth，后续接入）；public=任意已认证 user。
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityTeam    Visibility = "team"
	VisibilityPublic  Visibility = "public"
)

// BindingOwnerKind material 绑定的容器类型。
type BindingOwnerKind string

const (
	OwnerVirtualIP BindingOwnerKind = "virtual_ip"
	OwnerProduct   BindingOwnerKind = "product"
)

// Material curated 原料实体（asset_kind.resource_type=MATERIAL）。
// url（IMAGE/VIDEO）/ content（TEXT）至少一个非空。
// scope 创建后不可改；asset_kind 不可改。
type Material struct {
	ID          string
	Scope       string
	Name        string
	AssetKind   string // → asset_kinds.name (MATERIAL)
	URL         string
	Content     string
	OwnerUserID string
	Visibility  Visibility
	TeamID      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Binding material ↔ virtual_ip / material ↔ product 关联（N:N）。
type Binding struct {
	MaterialID string
	OwnerKind  BindingOwnerKind
	OwnerID    string
	BoundBy    string
	BoundAt    time.Time
}

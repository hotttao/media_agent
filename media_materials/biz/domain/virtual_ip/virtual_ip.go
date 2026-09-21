package virtual_ip

import "time"

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityTeam    Visibility = "team"
	VisibilityPublic  Visibility = "public"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

// Permission grant 权限档位。
type Permission string

const (
	PermissionViewer Permission = "viewer"
	PermissionEditor Permission = "editor"
)

// VirtualIp 虚拟形象实体（资源级 ACL：visibility + grants）。
// avatar_url/full_body_url/three_view_url/nine_view_url 是 IP 基础图（身份字段，非 asset）。
type VirtualIp struct {
	ID                  string
	OrganizationID      string
	Scope               string
	Name                string
	Description         string
	AvatarObjectPath    string
	FullBodyObjectPath  string
	ThreeViewObjectPath string
	NineViewObjectPath  string
	PersonaProfile      string
	Status              Status
	Visibility          Visibility
	OwnerUserID         string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Grant virtual_ip ↔ team 授权。
type Grant struct {
	OrganizationID string
	VirtualIPID    string
	TeamID         string
	Permission     Permission
	GrantedBy      string
	GrantedAt      time.Time
}

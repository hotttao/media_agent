package virtual_ip

import domain "media_agent/media_materials/biz/domain/virtual_ip"

type CreateCommand struct {
	Scope        string
	Name         string
	Description  string
	AvatarURL    string
	FullBodyURL  string
	ThreeViewURL string
	NineViewURL  string
	OwnerUserID  string
	Visibility   string
}

type UpdateCommand struct {
	ID           string
	Name         string
	Description  string
	AvatarURL    string
	FullBodyURL  string
	ThreeViewURL string
	NineViewURL  string
	CallerID     string
}

type GrantCommand struct {
	ID         string
	TeamID     string
	Permission string
	GrantedBy  string
}

type ListQuery = domain.ListQuery

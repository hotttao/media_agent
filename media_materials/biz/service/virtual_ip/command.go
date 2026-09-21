package virtual_ip

import domain "media_agent/media_materials/biz/domain/virtual_ip"

type CreateCommand struct {
	OrganizationID      string
	Scope               string
	Name                string
	Description         string
	AvatarObjectPath    string
	FullBodyObjectPath  string
	ThreeViewObjectPath string
	NineViewObjectPath  string
	PersonaProfile      string
	OwnerUserID         string
	Visibility          string
}

type UpdateCommand struct {
	ID                  string
	OrganizationID      string
	Name                string
	Description         string
	AvatarObjectPath    string
	FullBodyObjectPath  string
	ThreeViewObjectPath string
	NineViewObjectPath  string
	PersonaProfile      string
	CallerID            string
}

type GrantCommand struct {
	ID         string
	TeamID     string
	Permission string
	GrantedBy  string
}

type ListQuery = domain.ListQuery

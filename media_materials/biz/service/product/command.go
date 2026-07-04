package product

import domain "media_agent/media_materials/biz/domain/product"

type CreateCommand struct {
	Scope          string
	Name           string
	Description    string
	TargetAudience string
	ProductDetails string
	DisplayActions string
	CreatedBy      string
}

type UpdateCommand struct {
	ID             string
	Name           string
	Description    string
	TargetAudience string
	ProductDetails string
	DisplayActions string
	UpdatedBy      string
}

type ListQuery = domain.ListQuery

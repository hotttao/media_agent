package product

import domain "media_agent/media_materials/biz/domain/product"

type CreateCommand struct {
	OrganizationID   string
	Scope            string
	Name             string
	Description      string
	TargetAudience   string
	ProductDetails   string
	DisplayActions   string
	Tags             string
	ExternalCommerce string
	Metadata         string
	Images           []ImageCommand
	CreatedBy        string
}

type UpdateCommand struct {
	ID               string
	OrganizationID   string
	Name             string
	Description      string
	TargetAudience   string
	ProductDetails   string
	DisplayActions   string
	Tags             string
	ExternalCommerce string
	Metadata         string
	Images           []ImageCommand
	UpdatedBy        string
}

type ImageCommand struct {
	ObjectPath string
	IsMain     bool
	SortOrder  int
}

type ListQuery = domain.ListQuery

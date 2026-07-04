package product

import "time"

// Status product 状态。
type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

// Product 业务产品实体（无 user 归属、无 visibility，scope 级共享）。
// created_by / updated_by 仅审计，不作 ACL。任意已认证 user 可 CRUD。
type Product struct {
	ID             string
	Scope          string
	Name           string
	Description    string
	TargetAudience string
	ProductDetails string
	DisplayActions string
	Status         Status
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

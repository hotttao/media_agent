package shared

// Pagination 分页参数（应用级工具，与业务无关）。
type Pagination struct {
	Limit  int
	Offset int
}

// Normalize 规范化分页：limit 默认 50、上限 200，offset 非负。
func (p Pagination) Normalize() Pagination {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

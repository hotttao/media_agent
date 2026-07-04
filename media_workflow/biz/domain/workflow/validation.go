package workflow

// ValidationResult 闸1 校验结果。
type ValidationResult struct {
	Valid         bool
	Roots         []string
	OutputStepKey string
	DerivedEdges  []DerivedEdge
	InputSchema   []InputSchemaEntry
	RejectReasons []RejectReason
}

// DerivedEdge 推导的 DAG 边（step_output binding）。
type DerivedEdge struct {
	From    string // 源 step.key
	To      string // 目标 step.key
	ToInput string // 目标 slot.name
}

// RejectReason 校验失败原因。
type RejectReason struct {
	Code    string
	Message string
	Subject string
}

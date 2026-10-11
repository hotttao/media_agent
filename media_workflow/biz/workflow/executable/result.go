package executable

// StepResource is the common stable output of generated, restored and reused
// branches. It carries no expiring storage signature.
type StepResource struct {
	ID        string `json:"id"`
	AssetRef  string `json:"asset_ref"`
	URL       string `json:"url"`
	Content   string `json:"content,omitempty"`
	AssetKind string `json:"asset_kind"`
}
type StepOutput struct {
	OutputResourceID    string       `json:"output_resource_id"`
	OutputResource      StepResource `json:"output_resource"`
	ResultSource        string       `json:"result_source,omitempty"`
	ReusedFromStepRunID string       `json:"reused_from_step_run_id,omitempty"`
	ReuseEvidenceID     string       `json:"reuse_evidence_id,omitempty"`
}

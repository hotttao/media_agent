package job

import "time"

type JobStatus string

const (
	JobPending     JobStatus = "pending"
	JobRunning     JobStatus = "running"
	JobNeedConfirm JobStatus = "need_confirm"
	JobSucceeded   JobStatus = "succeeded"
	JobFailed      JobStatus = "failed"
	JobCancelled   JobStatus = "cancelled"
)

func (s JobStatus) IsTerminal() bool {
	return s == JobSucceeded || s == JobFailed || s == JobCancelled
}

type StepRunStatus string

const (
	SRBlocked     StepRunStatus = "blocked"
	SRPending     StepRunStatus = "pending"
	SRClaimed     StepRunStatus = "claimed"
	SRNeedConfirm StepRunStatus = "need_confirm"
	SRSucceeded   StepRunStatus = "succeeded"
	SRFailed      StepRunStatus = "failed"
	SRSkipped     StepRunStatus = "skipped"
)

// JobInput bundle（自包含，存 jobs.input jsonb）。
type JobInput struct {
	Product   *ProductData   `json:"product,omitempty"`
	VirtualIP *VirtualIpData `json:"virtual_ip,omitempty"`
	Assets    []AssetData    `json:"asset,omitempty"`
	Materials []MaterialData `json:"material,omitempty"`
}

type MaterialData struct{ Name, URL, Content string }
type AssetData struct{ Name, URL, Content string }
type ProductData struct {
	ID, Name, TargetAudience, ProductDetails, DisplayActions string
}
type VirtualIpData struct {
	ID, Name, AvatarURL, FullBodyURL, ThreeViewURL, NineViewURL string
}

type Candidate struct {
	Index     int    `json:"index"`
	URL       string `json:"url"`
	Content   string `json:"content"`
	IsPrimary bool   `json:"is_primary"`
}

type Job struct {
	ID              string
	Scope           string
	WorkflowID      string
	WorkflowVersion int
	Input           JobInput
	Status          JobStatus
	RetryCount      int
	RetryHistory    []RetryRecord
	SubmittedBy     string
	SubmittedAt     time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
	FailureReason   string
}

type RetryRecord struct {
	RetryAt       time.Time `json:"retry_at"`
	Reason        string    `json:"reason"`
	InputOverride *JobInput `json:"input_override,omitempty"`
}

type StepRun struct {
	ID              string
	JobID           string
	StepKey         string
	StepOrder       int
	TemplateID      string
	TemplateVersion int
	Status          StepRunStatus
	RetryCount      int
	ResolvedInputs  map[string]any
	RenderedPrompt  string
	Candidates      []Candidate
	SelectedIndex   *int
	OutputAssetID   string
	FailureReason   string
	ClaimedBy       string
	ClaimedAt       *time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
	Platform        string
	GenWay          string
	GenType         string
	AssetKind       string
	NeedsConfirm    bool
	CandidateCount  int
}

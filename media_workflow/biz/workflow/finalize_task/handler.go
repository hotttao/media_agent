package finalize_task

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const TaskDefinitionName = "media_finalize_step_v1"

var ErrApprovalRequired = errors.New("approval required")

type Candidate struct {
	URL     string `json:"url"`
	Content string `json:"content,omitempty"`
}

type Input struct {
	StepKey         string      `json:"step_key"`
	Candidates      []Candidate `json:"candidates"`
	OutputAssetKind string      `json:"output_asset_kind"`
	ApprovalPolicy  string      `json:"approval_policy"`
	ReusePolicy     string      `json:"reuse_policy"`
	JobID           string      `json:"job_id"`
}

type Resource struct {
	ID        string `json:"id"`
	AssetRef  string `json:"asset_ref"`
	URL       string `json:"url"`
	Content   string `json:"content,omitempty"`
	AssetKind string `json:"asset_kind"`
}

type Output struct {
	OutputResourceID string   `json:"output_resource_id"`
	OutputResource   Resource `json:"output_resource"`
}

type Handler struct{}

func (Handler) Execute(input Input) (Output, error) {
	if len(input.Candidates) == 0 {
		return Output{}, errors.New("capability returned no candidates")
	}
	if len(input.Candidates) > 1 || input.ApprovalPolicy == "always" {
		return Output{}, ErrApprovalRequired
	}
	candidate := input.Candidates[0]
	if candidate.URL == "" && candidate.Content == "" {
		return Output{}, errors.New("candidate url and content are both empty")
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s", input.JobID, input.StepKey, candidate.URL, candidate.Content)))
	id := "result_" + hex.EncodeToString(digest[:12])
	return Output{
		OutputResourceID: id,
		OutputResource:   Resource{ID: id, AssetRef: candidate.URL, URL: candidate.URL, Content: candidate.Content, AssetKind: input.OutputAssetKind},
	}, nil
}

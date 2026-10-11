package finalize_task

import (
	"errors"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/workflow/executable"
	"media_agent/media_workflow/biz/workflow/reuse_task"
)

const TaskDefinitionName = "media_finalize_step_v1"

var ErrApprovalRequired = errors.New("approval required")

type Candidate = domain.Candidate

type Input struct {
	StepKey             string               `json:"step_key"`
	Candidates          []Candidate          `json:"candidates"`
	OutputAssetKind     string               `json:"output_asset_kind"`
	ApprovalPolicy      string               `json:"approval_policy"`
	ReusePolicy         string               `json:"reuse_policy"`
	JobID               string               `json:"job_id"`
	SelectedCandidate   *Candidate           `json:"selected_candidate,omitempty"`
	FingerprintIdentity *reuse_task.Identity `json:"fingerprint_identity,omitempty"`
	ReuseResult         *reuse_task.Output   `json:"reuse_result,omitempty"`
}

type Resource = executable.StepResource
type Output = executable.StepOutput

func selectCandidate(input Input) (Candidate, error) {
	if len(input.Candidates) == 0 {
		return Candidate{}, errors.New("capability returned no candidates")
	}
	var selected Candidate
	if input.SelectedCandidate != nil {
		found := false
		for _, candidate := range input.Candidates {
			if candidate == *input.SelectedCandidate {
				selected, found = candidate, true
				break
			}
		}
		if !found {
			return Candidate{}, errors.New("selected candidate is not in the current generation")
		}
	} else {
		if len(input.Candidates) > 1 || input.ApprovalPolicy == "always" {
			return Candidate{}, ErrApprovalRequired
		}
		selected = input.Candidates[0]
	}
	if selected.URL == "" && selected.ObjectPath == "" && selected.Content == "" {
		return Candidate{}, errors.New("candidate is empty")
	}
	return selected, nil
}

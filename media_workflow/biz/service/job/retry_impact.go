package job

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/workflow/executable"
)

// loadFrozenRetryImpact never consults the editable Workflow or Templates.
// It validates the entire partition before any upstream rerun is authorized.
func (s *Service) loadFrozenRetryImpact(ctx context.Context, job *domain.Job, target string) (*executable.RetryImpact, []*domain.StepRun, error) {
	conflict := errors.New("retry source conflicts with frozen execution")
	c, err := s.compilations.GetRegistered(ctx, job.WorkflowID, job.WorkflowVersion)
	if err != nil || c == nil || c.WorkflowID != job.WorkflowID || c.WorkflowVersion != job.WorkflowVersion || c.DefinitionHash != job.CompilationHash {
		return nil, nil, conflict
	}
	var ir executable.ExecutableWorkflowIR
	decoder := json.NewDecoder(bytes.NewReader(c.IRJSON))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&ir) != nil || decoder.Decode(new(any)) != io.EOF || ir.WorkflowID != job.WorkflowID || ir.WorkflowVersion != job.WorkflowVersion {
		return nil, nil, conflict
	}
	impact, err := executable.PlanStepRetry(ir, target)
	if err != nil {
		return nil, nil, conflict
	}
	steps, err := s.stepRunRepo.ListByJob(ctx, job.ID)
	if err != nil {
		return nil, nil, errors.New("retry step snapshot unavailable")
	}
	if len(steps) != len(ir.Steps) {
		return nil, nil, conflict
	}
	known := make(map[string]bool, len(ir.Steps))
	for _, step := range ir.Steps {
		known[step.Key] = true
	}
	seen := make(map[string]bool, len(steps))
	for _, step := range steps {
		if step == nil || step.ID == "" || step.JobID != job.ID || !known[step.StepKey] || seen[step.StepKey] {
			return nil, nil, conflict
		}
		seen[step.StepKey] = true
	}
	return &impact, steps, nil
}

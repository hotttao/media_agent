package job

import (
	"context"
	"errors"
	"fmt"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/taskref"
)

func latestGeneration(execution *domain.EngineExecution) map[string]int {
	latest := map[string]int{}
	for _, task := range execution.Tasks {
		key, phase, iteration, ok := taskref.Parse(task.ReferenceTaskName)
		if !ok {
			continue
		}
		if phase == "loop" {
			switch value := task.Output["iteration"].(type) {
			case float64:
				iteration = int(value)
			case int:
				iteration = value
			}
		}
		if iteration > latest[key] {
			latest[key] = iteration
		}
	}
	return latest
}

func currentGeneration(task domain.EngineTask, latest map[string]int) bool {
	key, phase, iteration, ok := taskref.Parse(task.ReferenceTaskName)
	if !ok {
		return true // Legacy references and loop-external Finalize.
	}
	return phase != "loop" && phase != "gate" && iteration == latest[key]
}

func (s *Service) currentApprovalTask(ctx context.Context, job *domain.Job, sr *domain.StepRun) (*domain.EngineTask, int, error) {
	if job.Status == domain.JobCancelled {
		return nil, 0, errors.New("cannot review a terminal job")
	}
	execution, err := s.engine.Get(ctx, job.ConductorWorkflowID)
	if err != nil {
		return nil, 0, fmt.Errorf("load conductor execution: %w", err)
	}
	if execution == nil || execution.Status != "RUNNING" {
		return nil, 0, errors.New("workflow is not running; refresh before reviewing")
	}
	latest := latestGeneration(execution)
	var waiting *domain.EngineTask
	for i := range execution.Tasks {
		task := &execution.Tasks[i]
		key, phase, ok := parseTaskReference(task.ReferenceTaskName)
		if !ok || task.Retried || key != sr.StepKey || phase != "approval" || !currentGeneration(*task, latest) || task.Status != "IN_PROGRESS" {
			continue
		}
		if waiting != nil {
			return nil, 0, errors.New("ambiguous approval tasks; refresh before reviewing")
		}
		waiting = task
	}
	if waiting == nil {
		return nil, 0, errors.New("current generation is not waiting for approval")
	}
	_, _, iteration, _ := taskref.Parse(waiting.ReferenceTaskName)
	return waiting, iteration, nil
}

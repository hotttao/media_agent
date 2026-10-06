package job

import (
	"errors"

	domain "media_agent/media_workflow/biz/domain/job"
)

// Conductor rerun truncates tasks by sequence, not by business dependencies,
// and reactivates unsuccessful retained tasks. Until durable DAG recovery can
// restore unaffected branches, reject any rerun that could touch another step.
// This is a safety gate, not an implementation of parallel retry.
func validateStepRerunIsolation(execution *domain.EngineExecution, stepKey, renderID string) error {
	var sequence *int
	for _, task := range execution.Tasks {
		if task.TaskID == renderID {
			sequence = task.Sequence
			break
		}
	}
	if sequence == nil || *sequence < 0 {
		return errors.New("step retry requires authoritative task sequence")
	}
	for _, task := range execution.Tasks {
		key, _, known := parseTaskReference(task.ReferenceTaskName)
		if known && key == stepKey {
			continue
		}
		if !known {
			switch task.TaskType {
			case "FORK", "FORK_JOIN", "JOIN", "SWITCH", "DECISION":
				continue // Structural tasks do not own a StepRun.
			default:
				return errors.New("step retry cannot isolate an unknown executable task")
			}
		}
		if task.Sequence == nil || *task.Sequence >= *sequence || task.Status != "COMPLETED" {
			return errors.New("step retry would affect another branch; isolated DAG recovery is required")
		}
	}
	return nil
}

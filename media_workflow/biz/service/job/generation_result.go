package job

import (
	"context"
	"errors"

	domain "media_agent/media_workflow/biz/domain/job"
)

type generationSnapshotKey struct{}
type generationSnapshotsKey struct{}
type generationSnapshot struct {
	stepID, engineID string
	generation       int
	approvalIDs      map[string]bool
}

func withGenerationSnapshot(ctx context.Context, engineID string, step *domain.StepRun, execution *domain.EngineExecution) context.Context {
	return context.WithValue(ctx, generationSnapshotKey{}, makeGenerationSnapshot(engineID, step, execution))
}

func makeGenerationSnapshot(engineID string, step *domain.StepRun, execution *domain.EngineExecution) generationSnapshot {
	latest := latestGeneration(execution)
	snapshot := generationSnapshot{stepID: step.ID, engineID: engineID, generation: latest[step.StepKey], approvalIDs: map[string]bool{}}
	for _, task := range execution.Tasks {
		key, phase, ok := parseTaskReference(task.ReferenceTaskName)
		if ok && key == step.StepKey && phase == "approval" && currentGeneration(task, latest) && task.Status == "COMPLETED" {
			snapshot.approvalIDs[task.TaskID] = true
		}
	}
	return snapshot
}

// A newly accepted regeneration supersedes an older engine snapshot even before
// Conductor advances its loop, and does not increment Job.RetryCount. Recheck
// the local review boundary both before external registration and under Job→Step
// locks afterwards. Late registration remains an audit asset, never reattached.
func (s *Service) checkGenerationSnapshot(ctx context.Context, jobID, stepID string) error {
	if s.reviews == nil {
		return nil
	}
	snapshots := map[string]generationSnapshot{}
	if set, ok := ctx.Value(generationSnapshotsKey{}).(map[string]generationSnapshot); ok {
		if stepID == "" {
			snapshots = set
		} else if snapshot, exists := set[stepID]; exists {
			snapshots[stepID] = snapshot
		}
	}
	if snapshot, ok := ctx.Value(generationSnapshotKey{}).(generationSnapshot); ok && snapshot.stepID == stepID {
		snapshots[stepID] = snapshot
	}
	if len(snapshots) == 0 {
		return nil
	}
	reviews, err := s.reviews.LatestReviewsByJob(ctx, jobID)
	if err != nil {
		return err
	}
	for _, review := range reviews {
		snapshot, ok := snapshots[review.StepRunID]
		if !ok || review.ConductorWorkflowID != snapshot.engineID {
			continue
		}
		epoch := 0
		if execution, ok := ctx.Value(executionSnapshotKey{}).(executionSnapshot); ok {
			epoch = execution.retryCount
		}
		if review.AcceptedRetryCount != nil {
			if *review.AcceptedRetryCount < epoch {
				continue
			}
			if *review.AcceptedRetryCount > epoch {
				return errExecutionSnapshotStale
			}
		} else if epoch > 0 && review.Generation >= snapshot.generation {
			// Legacy unknown epoch: only the exact completed approval proves
			// compatibility. Never invent a cycle for an old audit record.
			if !snapshot.approvalIDs[review.ConductorTaskID] {
				return errExecutionSnapshotStale
			}
		}
		if review.Generation > snapshot.generation || review.Decision == "regenerate" && review.Generation >= snapshot.generation {
			return errExecutionSnapshotStale
		}
		if review.Generation == snapshot.generation && review.Decision == "accept" && !snapshot.approvalIDs[review.ConductorTaskID] {
			return errExecutionSnapshotStale
		}
	}
	return nil
}

// Finalize retries and Worker dispatch receipts are not generation identities.
// Only the uniquely successful capability invocation in the current round is.
func currentGenerationResultKey(engineID string, step *domain.StepRun, execution *domain.EngineExecution) (string, error) {
	if step == nil || execution == nil {
		return "", errors.New("generation snapshot unavailable")
	}
	latest := latestGeneration(execution)
	var successful *domain.EngineTask
	for i := range execution.Tasks {
		task := &execution.Tasks[i]
		key, phase, ok := parseTaskReference(task.ReferenceTaskName)
		if !ok || key != step.StepKey || phase != "invoke" || !currentGeneration(*task, latest) {
			continue
		}
		if task.Status == "IN_PROGRESS" || task.Status == "SCHEDULED" {
			return "", errors.New("generation capability invocation is still pending")
		}
		if task.Status != "COMPLETED" {
			continue
		}
		if successful != nil {
			return "", errors.New("generation has ambiguous successful capability invocations")
		}
		successful = task
	}
	if successful == nil {
		return "", errors.New("generation has no successful capability invocation")
	}
	return domain.GenerationResultKey(engineID, step.ID, successful.TaskID)
}

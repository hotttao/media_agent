package job

import domain "media_agent/media_workflow/biz/domain/job"

func nativeTaskFailed(status string) bool {
	return status == "FAILED" || status == "FAILED_WITH_TERMINAL_ERROR" || status == "TIMED_OUT"
}

// Reuses the already authorized/reconciled snapshot: no extra engine read or
// provenance write on GET, and no invented origin for frozen legacy outputs.
func annotateResultSources(snapshot *domain.EngineExecution, job *domain.Job, steps []*domain.StepRun) {
	for _, step := range steps {
		step.ResultSource, step.ReusedFromStepRunID = "", ""
	}
	if snapshot == nil || job.Status == domain.JobCancelled || (snapshot.WorkflowInstanceID != "" && snapshot.WorkflowInstanceID != job.ConductorWorkflowID) {
		return
	}
	latest := latestGeneration(snapshot)
	for _, task := range snapshot.Tasks {
		key, phase, ok := parseTaskReference(task.ReferenceTaskName)
		if !ok || phase != "finalize" || task.Retried || task.Status != "COMPLETED" || !currentGeneration(task, latest) {
			continue
		}
		id, _ := task.Output["output_resource_id"].(string)
		source, _ := task.Output["result_source"].(string)
		from, _ := task.Output["reused_from_step_run_id"].(string)
		for _, step := range steps {
			if step.StepKey != key || step.Status != domain.SRSucceeded || step.OutputAssetID == "" || step.OutputAssetID != id {
				continue
			}
			if source == "generated" && from == "" {
				step.ResultSource = source
			} else if source == "reused" && from != "" {
				step.ResultSource, step.ReusedFromStepRunID = source, from
			}
		}
	}
}

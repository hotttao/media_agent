package job

import (
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

func TestRerunConfirmationRequiresExactInputAndChangeEvidence(t *testing.T) {
	old := map[string]any{"job_id": "job", "inputs": map[string]any{"text": "old"}}
	input := map[string]any{"job_id": "job", "inputs": map[string]any{"text": "new"}}
	before := &domain.EngineExecution{WorkflowInstanceID: "engine", DefinitionName: "frozen", DefinitionVersion: 1, Status: "FAILED", Input: old, Tasks: []domain.EngineTask{{TaskID: "old-task"}}}
	for _, tc := range []struct {
		name      string
		uncertain bool
		mutate    func(*domain.EngineExecution)
		want      bool
	}{
		{"acknowledged", false, func(*domain.EngineExecution) {}, true},
		{"lost response changed input", true, func(*domain.EngineExecution) {}, true},
		{"wrong instance", false, func(e *domain.EngineExecution) { e.WorkflowInstanceID = "other" }, false},
		{"wrong definition", false, func(e *domain.EngineExecution) { e.DefinitionVersion = 2 }, false},
		{"different input", false, func(e *domain.EngineExecution) { e.Input = old }, false},
		{"cancelled", false, func(e *domain.EngineExecution) { e.Status = "TERMINATED" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after := &domain.EngineExecution{WorkflowInstanceID: "engine", DefinitionName: "frozen", DefinitionVersion: 1, Status: "RUNNING", Input: input, Tasks: []domain.EngineTask{{TaskID: "new-task"}}}
			tc.mutate(after)
			if got := confirmedRerun(before, after, input, tc.uncertain); got != tc.want {
				t.Fatalf("confirmation=%v want=%v", got, tc.want)
			}
		})
	}
	// RUNNING with already matching inputs is not proof of a lost rerun.
	after := *before
	after.Status = "RUNNING"
	if confirmedRerun(before, &after, old, true) {
		t.Fatal("mere RUNNING invented rerun acknowledgement")
	}
	after.Tasks = []domain.EngineTask{{TaskID: "replacement"}}
	if !confirmedRerun(before, &after, old, true) {
		t.Fatal("native replaced tasks were not recognized")
	}
}

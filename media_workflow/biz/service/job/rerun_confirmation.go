package job

import (
	"bytes"
	"encoding/json"

	domain "media_agent/media_workflow/biz/domain/job"
)

// A response-loss recovery must prove a change on the exact original instance,
// not merely observe RUNNING or matching input that was already present.
func confirmedRerun(before, after *domain.EngineExecution, input map[string]any, uncertain bool) bool {
	if before == nil || after == nil || after.WorkflowInstanceID != before.WorkflowInstanceID || !sameRerunInput(after.Input, input) {
		return false
	}
	if before.DefinitionName != after.DefinitionName || before.DefinitionVersion != after.DefinitionVersion {
		return false
	}
	switch after.Status {
	case "RUNNING", "PAUSED", "COMPLETED", "FAILED", "TIMED_OUT":
	default:
		return false
	}
	changedTasks := false
	current := map[string]bool{}
	for _, task := range after.Tasks {
		if task.TaskID != "" {
			current[task.TaskID] = true
		}
	}
	for _, task := range before.Tasks {
		if task.TaskID != "" && !current[task.TaskID] {
			changedTasks = true
			break
		}
	}
	changedInput := !sameRerunInput(before.Input, input)
	if uncertain {
		return changedInput || changedTasks
	}
	// A nominal success with the unchanged terminal snapshot is not proof.
	return changedInput || changedTasks || after.Status == "RUNNING" || after.Status == "PAUSED"
}

func sameRerunInput(left, right map[string]any) bool {
	a, aerr := canonicalRerunJSON(left)
	b, berr := canonicalRerunJSON(right)
	return aerr == nil && berr == nil && bytes.Equal(a, b)
}

// Struct fields retain declaration order while decoded maps sort keys on
// marshal. Compare the canonical wire objects, not those Go representations.
// UseNumber avoids silently rounding resource metadata during normalization.
func canonicalRerunJSON(value any) ([]byte, error) {
	wire, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(wire))
	decoder.UseNumber()
	var canonical any
	if err := decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

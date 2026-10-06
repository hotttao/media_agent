package executable

import (
	"errors"
	"sort"
)

var ErrInvalidRetryGraph = errors.New("frozen workflow retry graph is invalid")

// RetryBoundary identifies an unaffected output needed by a restarted slot.
// It contains business references, never Conductor sequence numbers or URLs.
type RetryBoundary struct {
	FromStep string `json:"from_step"`
	ToStep   string `json:"to_step"`
	ToSlot   string `json:"to_slot"`
}

// RetryImpact partitions a frozen graph. Preserved does not claim a node has a
// completed output: the recovery coordinator must validate runtime receipts.
type RetryImpact struct {
	TargetStep string          `json:"target_step"`
	Affected   []string        `json:"affected"`
	Preserved  []string        `json:"preserved"`
	Boundary   []RetryBoundary `json:"boundary"`
}

// PlanStepRetry traverses business dependencies, not editor order or engine
// task sequence. It is pure and cannot authorize a rerun or register a result.
func PlanStepRetry(ir ExecutableWorkflowIR, target string) (RetryImpact, error) {
	invalid := func() (RetryImpact, error) { return RetryImpact{}, ErrInvalidRetryGraph }
	if ir.SchemaVersion != SchemaVersion || ir.WorkflowID == "" || ir.WorkflowVersion <= 0 || len(ir.Steps) == 0 || target == "" {
		return invalid()
	}
	steps := make(map[string]ExecutableStepIR, len(ir.Steps))
	children := make(map[string][]string, len(ir.Steps))
	indegree := make(map[string]int, len(ir.Steps))
	for _, step := range ir.Steps {
		if step.Key == "" || step.Order <= 0 {
			return invalid()
		}
		if _, exists := steps[step.Key]; exists {
			return invalid()
		}
		steps[step.Key] = step
	}
	if _, exists := steps[target]; !exists {
		return invalid()
	}
	if _, exists := steps[ir.OutputStepKey]; !exists {
		return invalid()
	}
	for _, step := range ir.Steps {
		dependencies := make(map[string]bool, len(step.DependsOn))
		for _, parent := range step.DependsOn {
			if _, exists := steps[parent]; !exists || parent == step.Key || dependencies[parent] {
				return invalid()
			}
			dependencies[parent] = true
			children[parent] = append(children[parent], step.Key)
			indegree[step.Key]++
		}
		bindings := make(map[string]bool)
		for slot, value := range step.Context {
			if slot == "" {
				return invalid()
			}
			switch value.Source {
			case ValueStepOutput:
				if !dependencies[value.Ref] {
					return invalid()
				}
				bindings[value.Ref] = true
			case ValueWorkflowInput:
				if value.Ref == "" {
					return invalid()
				}
			case ValueConst:
			default:
				return invalid()
			}
		}
		// Hidden dependencies and missing dependency declarations both make
		// selective recovery unsafe, even if the graph itself is acyclic.
		if len(bindings) != len(dependencies) {
			return invalid()
		}
	}
	for key := range steps {
		if len(children[key]) == 0 && key != ir.OutputStepKey {
			return invalid()
		}
	}
	if len(children[ir.OutputStepKey]) != 0 {
		return invalid()
	}

	less := func(a, b string) bool {
		if steps[a].Order != steps[b].Order {
			return steps[a].Order < steps[b].Order
		}
		return a < b
	}
	ready := make([]string, 0, len(steps))
	for key := range steps {
		if indegree[key] == 0 {
			ready = append(ready, key)
		}
	}
	ordered := make([]string, 0, len(steps))
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
		key := ready[0]
		ready = ready[1:]
		ordered = append(ordered, key)
		for _, child := range children[key] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
			}
		}
	}
	if len(ordered) != len(steps) {
		return invalid()
	}
	affected := map[string]bool{target: true}
	for _, key := range ordered {
		if affected[key] {
			for _, child := range children[key] {
				affected[child] = true
			}
		}
	}
	impact := RetryImpact{TargetStep: target, Affected: []string{}, Preserved: []string{}, Boundary: []RetryBoundary{}}
	for _, key := range ordered {
		if !affected[key] {
			impact.Preserved = append(impact.Preserved, key)
			continue
		}
		impact.Affected = append(impact.Affected, key)
		slots := make([]string, 0, len(steps[key].Context))
		for slot := range steps[key].Context {
			slots = append(slots, slot)
		}
		sort.Strings(slots)
		for _, slot := range slots {
			value := steps[key].Context[slot]
			if value.Source == ValueStepOutput && !affected[value.Ref] {
				impact.Boundary = append(impact.Boundary, RetryBoundary{FromStep: value.Ref, ToStep: key, ToSlot: slot})
			}
		}
	}
	return impact, nil
}

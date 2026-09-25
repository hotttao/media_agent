// Package compiler converts a published business Workflow into a stable
// Conductor workflow definition. It deliberately has no Conductor client or
// database dependency; registration is an adapter concern.
package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	domain "media_agent/media_workflow/biz/domain/workflow"
)

const (
	StepWorkflowName    = "media_step_execution_v1"
	StepWorkflowVersion = 1
)

// Definition is the subset of a Conductor Workflow Definition owned by this
// compiler. Maps are intentionally used for input parameters because
// Conductor expressions are JSON values, not Go domain values.
type Definition struct {
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	Version          int            `json:"version"`
	Tasks            []Task         `json:"tasks"`
	InputParameters  map[string]any `json:"inputParameters,omitempty"`
	OutputParameters map[string]any `json:"outputParameters,omitempty"`
	SchemaVersion    int            `json:"schemaVersion"`
}

type Task struct {
	Name              string            `json:"name"`
	TaskReferenceName string            `json:"taskReferenceName"`
	Type              string            `json:"type"`
	InputParameters   map[string]any    `json:"inputParameters,omitempty"`
	SubWorkflowParam  *SubWorkflowParam `json:"subWorkflowParam,omitempty"`
	JoinOn            []string          `json:"joinOn,omitempty"`
	ForkTasks         [][]Task          `json:"forkTasks,omitempty"`
}

type SubWorkflowParam struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

type wave struct {
	steps []domain.Step
}

// Compile creates a deterministic Definition. Runtime step inputs are
// supplied by SubmitJob under workflow.input.wave_<n>_step_inputs; the
// definition never embeds a Job or StepRun ID.
func Compile(wf domain.Workflow) (Definition, error) {
	if strings.TrimSpace(wf.ID) == "" {
		return Definition{}, fmt.Errorf("workflow id is required")
	}
	if wf.Version <= 0 {
		return Definition{}, fmt.Errorf("workflow version must be positive")
	}
	if len(wf.Steps) == 0 {
		return Definition{}, fmt.Errorf("workflow must contain at least one step")
	}

	byKey := make(map[string]domain.Step, len(wf.Steps))
	indegree := make(map[string]int, len(wf.Steps))
	children := make(map[string][]string, len(wf.Steps))
	for _, step := range wf.Steps {
		key := strings.TrimSpace(step.Key)
		if key == "" {
			return Definition{}, fmt.Errorf("step key is required")
		}
		if _, exists := byKey[key]; exists {
			return Definition{}, fmt.Errorf("duplicate step key %q", key)
		}
		step.Key = key
		byKey[key] = step
		indegree[key] = 0
	}
	for _, step := range byKey {
		seen := make(map[string]struct{})
		for _, binding := range step.SlotBindings {
			if binding.Source != domain.SourceStepOutput {
				continue
			}
			ref := strings.TrimSpace(binding.Ref)
			if ref == "" {
				return Definition{}, fmt.Errorf("step %q has empty step_output reference", step.Key)
			}
			if _, exists := byKey[ref]; !exists {
				return Definition{}, fmt.Errorf("step %q references unknown step %q", step.Key, ref)
			}
			if _, duplicate := seen[ref]; duplicate {
				continue
			}
			seen[ref] = struct{}{}
			indegree[step.Key]++
			children[ref] = append(children[ref], step.Key)
		}
	}

	waves, err := topologicalWaves(byKey, indegree, children)
	if err != nil {
		return Definition{}, err
	}
	if strings.TrimSpace(wf.OutputStepKey) == "" {
		return Definition{}, fmt.Errorf("output step key is required")
	}
	if _, exists := byKey[wf.OutputStepKey]; !exists {
		return Definition{}, fmt.Errorf("output step %q does not exist", wf.OutputStepKey)
	}
	if len(children[wf.OutputStepKey]) != 0 {
		return Definition{}, fmt.Errorf("output step %q is not a sink", wf.OutputStepKey)
	}
	if len(waves[len(waves)-1].steps) != 1 || waves[len(waves)-1].steps[0].Key != wf.OutputStepKey {
		return Definition{}, fmt.Errorf("output step %q must be the only final wave step", wf.OutputStepKey)
	}

	definition := Definition{
		Name:             "media_workflow_" + sanitize(wf.ID),
		Description:      wf.Description,
		Version:          wf.Version,
		InputParameters:  map[string]any{},
		OutputParameters: map[string]any{},
		SchemaVersion:    2,
	}
	for index, current := range waves {
		inputRef := fmt.Sprintf("${workflow.input.wave_%d_step_inputs}", index)
		if len(current.steps) == 1 {
			step := current.steps[0]
			ref := fmt.Sprintf("wave_%d__%s", index, sanitize(step.Key))
			definition.Tasks = append(definition.Tasks, Task{
				Name:              "step_execution_" + sanitize(step.Key),
				TaskReferenceName: ref,
				Type:              "SUB_WORKFLOW",
				InputParameters:   map[string]any{"workflowInput": inputRef + "[0]"},
				SubWorkflowParam:  &SubWorkflowParam{Name: StepWorkflowName, Version: StepWorkflowVersion},
			})
			continue
		}
		definition.Tasks = append(definition.Tasks,
			Task{
				Name:              fmt.Sprintf("wave_%d_dynamic_fork", index),
				TaskReferenceName: fmt.Sprintf("wave_%d_fork", index),
				Type:              "FORK_JOIN_DYNAMIC",
				InputParameters: map[string]any{
					"forkTaskWorkflow":        StepWorkflowName,
					"forkTaskWorkflowVersion": StepWorkflowVersion,
					"forkTaskInputs":          inputRef,
				},
			},
			Task{
				Name:              fmt.Sprintf("wave_%d_join", index),
				TaskReferenceName: fmt.Sprintf("wave_%d_join", index),
				Type:              "JOIN",
				JoinOn:            []string{},
			},
		)
	}
	definition.OutputParameters["output_resource_id"] = fmt.Sprintf("${%s.output.output_resource_id}", definition.Tasks[len(definition.Tasks)-1].TaskReferenceName)
	return definition, nil
}

func topologicalWaves(byKey map[string]domain.Step, indegree map[string]int, children map[string][]string) ([]wave, error) {
	remaining := len(byKey)
	var waves []wave
	for remaining > 0 {
		ready := make([]domain.Step, 0)
		for key, degree := range indegree {
			if degree == 0 {
				ready = append(ready, byKey[key])
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("workflow contains a cycle")
		}
		sort.Slice(ready, func(i, j int) bool {
			if ready[i].Order != ready[j].Order {
				return ready[i].Order < ready[j].Order
			}
			return ready[i].Key < ready[j].Key
		})
		waves = append(waves, wave{steps: ready})
		for _, step := range ready {
			delete(indegree, step.Key)
			remaining--
			for _, child := range children[step.Key] {
				indegree[child]--
			}
		}
	}
	return waves, nil
}

func sanitize(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "workflow"
	}
	return b.String()
}

// CanonicalJSON is used for definition_hash and Conductor conflict checks.
func CanonicalJSON(def Definition) ([]byte, error) { return json.Marshal(def) }

func DefinitionHash(def Definition) (string, error) {
	b, err := CanonicalJSON(def)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

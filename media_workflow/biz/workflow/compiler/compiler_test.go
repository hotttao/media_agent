package compiler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	domain "media_agent/media_workflow/biz/domain/workflow"
)

func TestCompileDiamondUsesWaveBarriers(t *testing.T) {
	wf := domain.Workflow{
		ID: "wf_demo", Version: 3, OutputStepKey: "e",
		Steps: []domain.Step{
			{Key: "a", Order: 1},
			{Key: "b", Order: 2},
			{Key: "c", Order: 3, SlotBindings: []domain.SlotBinding{{Source: domain.SourceStepOutput, Ref: "a"}}},
			{Key: "d", Order: 4, SlotBindings: []domain.SlotBinding{{Source: domain.SourceStepOutput, Ref: "b"}}},
			{Key: "e", Order: 5, SlotBindings: []domain.SlotBinding{{Source: domain.SourceStepOutput, Ref: "c"}, {Source: domain.SourceStepOutput, Ref: "d"}}},
		},
	}
	def, err := Compile(wf)
	require.NoError(t, err)
	require.Len(t, def.Tasks, 5)
	require.Equal(t, "FORK_JOIN_DYNAMIC", def.Tasks[0].Type)
	require.Equal(t, "JOIN", def.Tasks[1].Type)
	require.Equal(t, "FORK_JOIN_DYNAMIC", def.Tasks[2].Type)
	require.Equal(t, "JOIN", def.Tasks[3].Type)
	require.Equal(t, "SUB_WORKFLOW", def.Tasks[4].Type)
	require.Equal(t, StepWorkflowName, def.Tasks[0].InputParameters["forkTaskWorkflow"])
	require.Equal(t, "${workflow.input.wave_1_step_inputs}", def.Tasks[2].InputParameters["forkTaskInputs"])
}

func TestCompileRejectsCycleAndNonSinkOutput(t *testing.T) {
	_, err := Compile(domain.Workflow{ID: "wf", Version: 1, OutputStepKey: "a", Steps: []domain.Step{
		{Key: "a", SlotBindings: []domain.SlotBinding{{Source: domain.SourceStepOutput, Ref: "b"}}},
		{Key: "b", SlotBindings: []domain.SlotBinding{{Source: domain.SourceStepOutput, Ref: "a"}}},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cycle")

	_, err = Compile(domain.Workflow{ID: "wf", Version: 1, OutputStepKey: "a", Steps: []domain.Step{
		{Key: "a"}, {Key: "b", SlotBindings: []domain.SlotBinding{{Source: domain.SourceStepOutput, Ref: "a"}}},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a sink")
}

func TestCompileHashIsStable(t *testing.T) {
	wf := domain.Workflow{ID: "wf:demo", Version: 1, OutputStepKey: "a", Steps: []domain.Step{{Key: "a"}}}
	one, err := Compile(wf)
	require.NoError(t, err)
	two, err := Compile(wf)
	require.NoError(t, err)
	h1, err := DefinitionHash(one)
	require.NoError(t, err)
	h2, err := DefinitionHash(two)
	require.NoError(t, err)
	require.Equal(t, h1, h2)
	require.True(t, strings.HasPrefix(one.Name, "media_workflow_wf_demo"))
}

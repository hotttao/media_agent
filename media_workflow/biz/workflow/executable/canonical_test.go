package executable

import (
	"testing"

	"github.com/stretchr/testify/require"
	template "media_agent/media_workflow/biz/domain/template"
)

func TestHashIsStableAcrossMapInsertionOrder(t *testing.T) {
	one := ExecutableWorkflowIR{SchemaVersion: SchemaVersion, WorkflowID: "wf", WorkflowVersion: 1, OutputStepKey: "s", Steps: []ExecutableStepIR{{
		Key: "s", Context: map[string]ValueRef{
			"person":  {Source: ValueWorkflowInput, Ref: "person"},
			"garment": {Source: ValueWorkflowInput, Ref: "garment"},
		}, Invocation: template.InvocationMapping{
			"prompt": {Op: template.InvocationRenderTemplate},
			"images": {Op: template.InvocationList, Items: []template.InvocationExpression{{Op: template.InvocationAssetRef, Slot: "person"}}},
		},
	}}}
	two := ExecutableWorkflowIR{SchemaVersion: SchemaVersion, WorkflowID: "wf", WorkflowVersion: 1, OutputStepKey: "s", Steps: []ExecutableStepIR{{
		Key: "s", Context: map[string]ValueRef{
			"garment": {Source: ValueWorkflowInput, Ref: "garment"},
			"person":  {Source: ValueWorkflowInput, Ref: "person"},
		}, Invocation: template.InvocationMapping{
			"images": {Op: template.InvocationList, Items: []template.InvocationExpression{{Op: template.InvocationAssetRef, Slot: "person"}}},
			"prompt": {Op: template.InvocationRenderTemplate},
		},
	}}}

	hashOne, err := Hash(one)
	require.NoError(t, err)
	hashTwo, err := Hash(two)
	require.NoError(t, err)
	require.Equal(t, hashOne, hashTwo)
}

package render_task

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerExecutesStableRendererContract(t *testing.T) {
	output, err := (Handler{}).Execute(Input{
		Template: TemplateInput{ID: "tpl", Version: 2, Body: "hello {{.subject.name}}"},
		Context:  map[string]any{"subject": map[string]any{"name": "world"}},
	})
	require.NoError(t, err)
	require.Equal(t, "hello world", output.Prompt)
	require.NotEmpty(t, output.PromptDigest)
}

package template

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderUsesDynamicJSONContextDeterministically(t *testing.T) {
	req := RenderRequest{
		TemplateID: "tpl_model", TemplateVersion: 3,
		Body: "保持人物 {{.person.description}} 的特征，穿上 {{.garment.name}}。",
		Context: map[string]any{
			"person":  map[string]any{"description": "小雅"},
			"garment": map[string]any{"name": "白色连衣裙"},
		},
	}

	one, err := Render(req)
	require.NoError(t, err)
	two, err := Render(req)
	require.NoError(t, err)
	require.Equal(t, "保持人物 小雅 的特征，穿上 白色连衣裙。", one.Prompt)
	require.Equal(t, one.PromptDigest, two.PromptDigest)
	require.Contains(t, one.PromptDigest, "sha256:")
}

func TestRenderRejectsMissingContextAndInvalidTemplate(t *testing.T) {
	_, err := Render(RenderRequest{Body: "{{.person.name}}", Context: map[string]any{}})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrTemplateRender))

	_, err = Render(RenderRequest{Body: "{{", Context: map[string]any{}})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidTemplate))
}

func TestRenderRejectsNonJSONContext(t *testing.T) {
	_, err := Render(RenderRequest{Body: "ok", Context: map[string]any{"bad": func() {}}})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidRenderContext))
}

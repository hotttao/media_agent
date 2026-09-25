// Package render_task implements the stable application contract used by the
// media_render_template_v1 Conductor worker. Polling and acknowledgement belong
// to the Conductor adapter; this package remains transport-independent.
package render_task

import template "media_agent/media_workflow/biz/domain/template"

const TaskDefinitionName = "media_render_template_v1"

type TemplateInput struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Body    string `json:"body"`
}

type Input struct {
	Template TemplateInput  `json:"template"`
	Context  map[string]any `json:"context"`
}

type Output struct {
	Prompt       string `json:"prompt"`
	PromptDigest string `json:"prompt_digest"`
}

type Handler struct{}

func (Handler) Execute(input Input) (Output, error) {
	result, err := template.Render(template.RenderRequest{
		TemplateID: input.Template.ID, TemplateVersion: input.Template.Version,
		Body: input.Template.Body, Context: input.Context,
	})
	if err != nil {
		return Output{}, err
	}
	return Output{Prompt: result.Prompt, PromptDigest: result.PromptDigest}, nil
}

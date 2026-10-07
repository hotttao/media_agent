// Package render_task implements the stable application contract used by the
// media_render_template_v1 Conductor worker. Polling and acknowledgement belong
// to the Conductor adapter; this package remains transport-independent.
package render_task

import (
	"errors"
	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared/taskerror"
)

const TaskDefinitionName = "media_render_template_v1"
const FeedbackMode = "last_regeneration_v1"

type PreviousReview struct {
	Decision string `json:"decision"`
	Feedback string `json:"feedback"`
}

type TemplateInput struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Body    string `json:"body"`
}

type Input struct {
	Template           TemplateInput   `json:"template"`
	Context            map[string]any  `json:"context"`
	FeedbackMode       string          `json:"feedback_mode"`
	Generation         int             `json:"generation"`
	PreviousGeneration int             `json:"previous_generation"`
	PreviousReview     *PreviousReview `json:"previous_review"`
}

type Output struct {
	Prompt       string `json:"prompt"`
	PromptDigest string `json:"prompt_digest"`
}

type Handler struct{}

func (Handler) Execute(input Input) (Output, error) {
	feedback := ""
	if input.FeedbackMode != "" {
		if input.FeedbackMode != FeedbackMode || input.Generation < 1 {
			return Output{}, taskerror.WrapPermanent("renderer_feedback_invalid", errors.New("invalid renderer feedback contract"))
		}
		if input.Generation == 1 {
			if input.PreviousGeneration != 0 || input.PreviousReview != nil {
				return Output{}, taskerror.WrapPermanent("renderer_feedback_invalid", errors.New("first generation cannot reuse a review"))
			}
		} else {
			if input.PreviousGeneration != input.Generation-1 || input.PreviousReview == nil || input.PreviousReview.Decision != "regenerate" {
				return Output{}, taskerror.WrapPermanent("renderer_feedback_invalid", errors.New("renderer review is not from the previous generation"))
			}
			feedback = input.PreviousReview.Feedback
		}
	} else if input.PreviousReview != nil {
		return Output{}, taskerror.WrapPermanent("renderer_feedback_invalid", errors.New("renderer feedback mode required"))
	}
	result, err := template.Render(template.RenderRequest{
		TemplateID: input.Template.ID, TemplateVersion: input.Template.Version,
		Body: input.Template.Body, Context: input.Context, Feedback: feedback,
	})
	if err != nil {
		return Output{}, taskerror.WrapPermanent("renderer_template_invalid", err)
	}
	return Output{Prompt: result.Prompt, PromptDigest: result.PromptDigest}, nil
}

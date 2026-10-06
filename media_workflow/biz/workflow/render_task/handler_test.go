package render_task

import (
	"crypto/sha256"
	"fmt"
	"strings"
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

func TestFeedbackRendersAsPlainTextAndChangesDigest(t *testing.T) {
	input := Input{Template: TemplateInput{ID: "tpl", Version: 2, Body: "hello {{.subject}}"}, Context: map[string]any{"subject": "world"}, FeedbackMode: FeedbackMode, Generation: 1}
	first, err := (Handler{}).Execute(input)
	require.NoError(t, err)
	require.Equal(t, "hello world", first.Prompt)
	input.Generation, input.PreviousGeneration = 2, 1
	input.PreviousReview = &PreviousReview{Decision: "regenerate", Feedback: "换成蓝色 {{.missing}}\n保留背景"}
	second, err := (Handler{}).Execute(input)
	require.NoError(t, err)
	require.Equal(t, "hello world\n\n用户重新生成反馈（本轮）：\n换成蓝色 {{.missing}}\n保留背景", second.Prompt)
	require.Equal(t, fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(second.Prompt))), second.PromptDigest)
	require.NotEqual(t, first.PromptDigest, second.PromptDigest)
	again, err := (Handler{}).Execute(input)
	require.NoError(t, err)
	require.Equal(t, second, again)
	input.Generation, input.PreviousGeneration = 3, 2
	input.PreviousReview.Feedback = "改为红色"
	third, err := (Handler{}).Execute(input)
	require.NoError(t, err)
	require.Contains(t, third.Prompt, "改为红色")
	require.NotContains(t, third.Prompt, "蓝色")
}

func TestRendererRejectsInvalidReviewGeneration(t *testing.T) {
	for _, input := range []Input{
		{FeedbackMode: "unknown", Generation: 1},
		{FeedbackMode: FeedbackMode, Generation: 0},
		{FeedbackMode: FeedbackMode, Generation: 1, PreviousGeneration: 1, PreviousReview: &PreviousReview{Decision: "regenerate"}},
		{FeedbackMode: FeedbackMode, Generation: 3, PreviousGeneration: 1, PreviousReview: &PreviousReview{Decision: "regenerate", Feedback: "old"}},
		{FeedbackMode: FeedbackMode, Generation: 2, PreviousGeneration: 1, PreviousReview: &PreviousReview{Decision: "accept"}},
		{FeedbackMode: FeedbackMode, Generation: 2, PreviousGeneration: 1},
		{PreviousReview: &PreviousReview{Decision: "regenerate", Feedback: "unsupported"}},
		{FeedbackMode: FeedbackMode, Generation: 2, PreviousGeneration: 1, PreviousReview: &PreviousReview{Decision: "regenerate", Feedback: strings.Repeat("x", 8193)}},
	} {
		input.Template = TemplateInput{ID: "tpl", Version: 1, Body: "base"}
		_, err := (Handler{}).Execute(input)
		require.Error(t, err)
	}
}

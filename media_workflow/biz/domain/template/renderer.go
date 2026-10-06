package template

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	texttemplate "text/template"
)

var (
	ErrInvalidRenderContext = errors.New("invalid render context")
	ErrInvalidTemplate      = errors.New("invalid template")
	ErrTemplateRender       = errors.New("template render failed")
)

type RenderRequest struct {
	TemplateID      string         `json:"template_id"`
	TemplateVersion int            `json:"template_version"`
	Body            string         `json:"body"`
	Context         map[string]any `json:"context"`
	Feedback        string         `json:"feedback,omitempty"`
}

type RenderResult struct {
	Prompt       string `json:"prompt"`
	PromptDigest string `json:"prompt_digest"`
}

// Render evaluates a template against JSON-only context. Normalizing through
// JSON prevents callers from exposing Go methods or executable values to the
// text/template engine. No custom functions are registered.
func Render(req RenderRequest) (RenderResult, error) {
	if len(req.Feedback) > 8192 {
		return RenderResult{}, ErrInvalidRenderContext
	}
	contextJSON, err := json.Marshal(req.Context)
	if err != nil {
		return RenderResult{}, fmt.Errorf("%w: %v", ErrInvalidRenderContext, err)
	}
	var safeContext map[string]any
	if err := json.Unmarshal(contextJSON, &safeContext); err != nil {
		return RenderResult{}, fmt.Errorf("%w: %v", ErrInvalidRenderContext, err)
	}

	tmpl, err := texttemplate.New("prompt").Option("missingkey=error").Parse(req.Body)
	if err != nil {
		return RenderResult{}, fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, safeContext); err != nil {
		return RenderResult{}, fmt.Errorf("%w: %v", ErrTemplateRender, err)
	}

	prompt := rendered.String()
	if req.Feedback != "" {
		// Feedback is data, not template source. Do not parse or execute it.
		prompt += "\n\n用户重新生成反馈（本轮）：\n" + req.Feedback
	}
	digest := sha256.Sum256([]byte(prompt))
	return RenderResult{Prompt: prompt, PromptDigest: "sha256:" + hex.EncodeToString(digest[:])}, nil
}

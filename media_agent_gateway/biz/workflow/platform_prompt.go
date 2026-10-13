package workflow

import (
	"bytes"
	"embed"
	"errors"
	"media_agent/media_agent_gateway/biz/shared/client"
	"strconv"
	"text/template"
)

//go:embed templates/platform_prompt_v2.md
var platformTemplates embed.FS
var ErrPromptTemplate = errors.New("unsupported platform prompt template")

func RenderPlatformPrompt(v client.MetadataContext) (string, string, error) {
	version := v.PlatformPromptTemplateVersion
	if version == "" {
		version = "1"
	}
	if version == "1" {
		return "You are working in workspace " + v.WorkspaceID + ".\nOnly /workspace persists. Keep work inside /workspace.\nRuntime configuration and skills are pinned to this session's selected version.\n", version, nil
	}
	if version != "2" {
		return "", "", ErrPromptTemplate
	}
	source, e := platformTemplates.ReadFile("templates/platform_prompt_v2.md")
	if e != nil {
		return "", "", e
	}
	tpl, e := template.New("platform").Funcs(template.FuncMap{"quote": strconv.Quote}).Parse(string(source))
	if e != nil {
		return "", "", e
	}
	var result bytes.Buffer
	e = tpl.Execute(&result, v)
	return result.String(), version, e
}

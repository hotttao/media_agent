package media_management

import (
	"errors"
	contract "media_agent/media_worker/biz/shared/media_management"
	"path"
	"strings"
)

var ErrInvalid = errors.New("invalid media tool parameters")
var Names = []string{"templates_list", "templates_get", "templates_create", "templates_publish", "workflows_list", "workflows_get", "workflows_create", "workflows_validate", "workflows_publish", "jobs_list", "jobs_get", "jobs_submit", "capabilities_list"}

func Known(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}
func ReadOnly(name string) bool {
	return strings.HasSuffix(name, "_list") || strings.HasSuffix(name, "_get") || name == "workflows_validate"
}
func NeedsWorkspace(name string) bool { return name == "workflows_create" || name == "jobs_submit" }
func Supported(key string, version int) bool {
	return version == 1 && (key == "doubao.i2v.first_frame_prompt" || key == "gpt.t2i.text_prompt" || key == "gpt.i2i.multi_image_prompt")
}
func Validate(name string, p contract.Params) error {
	if !Known(name) {
		return ErrInvalid
	}
	if strings.HasSuffix(name, "_list") {
		if p.Page < 1 || p.Page > 2147483647 || p.PageSize < 1 || p.PageSize > 100 {
			return ErrInvalid
		}
	}
	if (strings.HasSuffix(name, "_get") || strings.HasSuffix(name, "_publish")) && p.ID == "" {
		return ErrInvalid
	}
	if p.Version < 0 {
		return ErrInvalid
	}
	switch name {
	case "templates_create":
		if p.Name == "" || p.Scope == "" || p.PromptBody == "" || p.OutputAssetKind == "" || !Supported(p.CapabilityKey, p.CapabilityVersion) {
			return ErrInvalid
		}
	case "workflows_create", "workflows_validate":
		if p.Scope == "" || p.OutputStepKey == "" || len(p.Steps) == 0 {
			return ErrInvalid
		}
		if name == "workflows_create" && p.Name == "" {
			return ErrInvalid
		}
	case "jobs_submit":
		if p.WorkflowID == "" || p.WorkflowVersion < 1 || len(p.IdempotencyKey) == 0 || len(p.IdempotencyKey) > 160 {
			return ErrInvalid
		}
		for _, items := range [][]contract.MediaInput{p.Input.Assets, p.Input.Materials} {
			seen := map[string]bool{}
			for _, item := range items {
				if item.Name == "" || item.AssetKind == "" || seen[item.Name] || (item.ObjectPath == "") == (item.Content == "") {
					return ErrInvalid
				}
				seen[item.Name] = true
				if item.ObjectPath != "" && (strings.Contains(item.ObjectPath, ":") || strings.HasPrefix(item.ObjectPath, "/") || path.Clean(item.ObjectPath) != item.ObjectPath || strings.ContainsAny(item.ObjectPath, "\\\x00")) {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}
func DuplicateKey(name string, p contract.Params) string {
	if strings.HasSuffix(name, "_publish") {
		return p.ID
	}
	if name == "jobs_submit" {
		return p.IdempotencyKey
	}
	return ""
}

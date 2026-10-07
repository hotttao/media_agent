package worker_gateway

import (
	"fmt"
	"strings"

	capability "media_agent/media_workflow/biz/domain/capability"
)

// Validate against the published immutable Provider signature, not business
// Step names. Storage ownership, expiry and MIME are separate checks.
func validateProviderInputs(contract *capability.Capability, input map[string]any) error {
	if contract.OutputMediaType == "" {
		return nil
	}
	if contract.ValidateContract() != nil {
		return ErrInvalidTask
	}
	if input["capability_key"] != contract.Key || fmt.Sprint(input["capability_version"]) != fmt.Sprint(contract.Version) {
		return ErrInvalidTask
	}
	for _, field := range contract.Inputs {
		value, exists := input[field.Name]
		if !exists || value == nil {
			if field.Required {
				return ErrInvalidTask
			}
			continue
		}
		switch field.Type {
		case capability.InputString:
			text, ok := value.(string)
			if !ok || (field.Required && strings.TrimSpace(text) == "") {
				return ErrInvalidTask
			}
		case capability.InputResourceRef:
			ref, ok := value.(string)
			if !ok || strings.TrimSpace(ref) == "" {
				return ErrInvalidTask
			}
		case capability.InputResourceRefList:
			var refs []string
			switch values := value.(type) {
			case []string:
				refs = values
			case []any:
				for _, item := range values {
					ref, ok := item.(string)
					if !ok {
						return ErrInvalidTask
					}
					refs = append(refs, ref)
				}
			default:
				return ErrInvalidTask
			}
			if !field.AcceptsResourceCount(len(refs)) {
				return ErrInvalidTask
			}
			for _, ref := range refs {
				if strings.TrimSpace(ref) == "" {
					return ErrInvalidTask
				}
			}
		default:
			return ErrInvalidTask
		}
	}
	return nil
}

package execution

import (
	"errors"
	"fmt"
	"strings"

	"media_agent/media_worker/biz/domain/execution"
)

func SelectCapabilities(available []execution.Capability, raw string) ([]execution.Capability, error) {
	if strings.TrimSpace(raw) == "" {
		return available, nil
	}
	byRef := make(map[string]execution.Capability, len(available))
	for _, capability := range available {
		byRef[fmt.Sprintf("%s@%d", capability.Key, capability.Version)] = capability
	}
	selected := make([]execution.Capability, 0)
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		ref := strings.TrimSpace(item)
		capability, ok := byRef[ref]
		if !ok {
			return nil, fmt.Errorf("unsupported capability %q", ref)
		}
		if !seen[ref] {
			selected = append(selected, capability)
			seen[ref] = true
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("at least one capability is required")
	}
	return selected, nil
}

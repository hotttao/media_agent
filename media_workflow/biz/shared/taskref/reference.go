// Package taskref encodes loop-safe Conductor task references without losing
// the original business key. Conductor reserves "__" for iteration suffixes.
package taskref

import (
	"encoding/hex"
	"strconv"
	"strings"
)

func Reference(stepKey, phase string) string {
	return "step_" + hex.EncodeToString([]byte(stepKey)) + "_" + phase
}

// Parse accepts only the new loop-safe format, optionally suffixed by the
// positive iteration number appended by Conductor. Old references are parsed
// separately by the compatibility adapter.
func Parse(reference string) (key, phase string, iteration int, ok bool) {
	base := reference
	if before, after, found := strings.Cut(reference, "__"); found {
		var err error
		iteration, err = strconv.Atoi(after)
		if err != nil || iteration <= 0 {
			return "", "", 0, false
		}
		base = before
	}
	if !strings.HasPrefix(base, "step_") {
		return "", "", 0, false
	}
	encoded, phase, found := strings.Cut(strings.TrimPrefix(base, "step_"), "_")
	if !found || encoded == "" {
		return "", "", 0, false
	}
	switch phase {
	case "render", "invoke", "approval", "gate", "loop", "prepare", "reuse", "reuse_gate":
	default:
		return "", "", 0, false
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil || len(decoded) == 0 {
		return "", "", 0, false
	}
	return string(decoded), phase, iteration, true
}

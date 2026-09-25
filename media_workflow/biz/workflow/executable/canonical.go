package executable

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CanonicalJSON is deterministic for this IR because encoding/json sorts map
// keys and the semantic compiler sorts steps and dependencies.
func CanonicalJSON(ir ExecutableWorkflowIR) ([]byte, error) {
	return json.Marshal(ir)
}

func Hash(ir ExecutableWorkflowIR) (string, error) {
	canonical, err := CanonicalJSON(ir)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

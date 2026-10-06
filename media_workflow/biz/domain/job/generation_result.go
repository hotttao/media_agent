package job

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// GenerationResultKey identifies the accepted capability result, not a
// Finalize retry, delivery attempt or content checksum.
func GenerationResultKey(engineID, stepRunID, successfulTaskID string) (string, error) {
	for _, value := range []string{engineID, stepRunID, successfulTaskID} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || len(value) > 256 {
			return "", errors.New("invalid generation result identity")
		}
	}
	encoded, err := json.Marshal([]string{engineID, stepRunID, successfulTaskID})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

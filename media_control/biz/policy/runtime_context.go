package policy

import "crypto/subtle"

func AllowsRuntimeContext(provided, configured string) bool {
	if provided == "" || configured == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(configured)) == 1
}

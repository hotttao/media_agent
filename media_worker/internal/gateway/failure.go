package gateway

import "errors"

// Executors can classify failures without depending on server packages.
// Unclassified fixture errors preserve the existing terminal behavior.
func FailureKind(err error) string {
	var classified interface{ Permanent() bool }
	if errors.As(err, &classified) && !classified.Permanent() {
		return "transient"
	}
	return "permanent"
}

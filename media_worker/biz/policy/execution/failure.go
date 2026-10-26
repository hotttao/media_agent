package execution

import (
	"errors"

	"media_agent/media_worker/biz/domain/execution"
)

// Explicit non-permanent failures are retryable; other errors retain terminal behavior.
func FailureKind(err error) string {
	var c interface{ Permanent() bool }
	if errors.As(err, &c) && !c.Permanent() {
		return "transient"
	}
	return "permanent"
}
func ResultRejected(err error) bool { return errors.Is(err, execution.ErrResultRejected) }

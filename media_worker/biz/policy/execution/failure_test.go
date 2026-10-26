package execution

import (
	"errors"
	"fmt"
	"testing"
)

type executorFailure bool

func (e executorFailure) Error() string   { return "provider failed" }
func (e executorFailure) Permanent() bool { return bool(e) }

func TestExecutorClassificationIncludesWrappedFailures(t *testing.T) {
	for _, tc := range []struct {
		err  error
		kind string
	}{
		{errors.New("fixture failure"), "permanent"},
		{executorFailure(true), "permanent"},
		{fmt.Errorf("wrapped: %w", executorFailure(false)), "transient"},
	} {
		if got := FailureKind(tc.err); got != tc.kind {
			t.Fatalf("kind=%s want=%s", got, tc.kind)
		}
	}
}

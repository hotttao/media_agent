package workeridentity

import "testing"

func TestStatusCannotReactivateOrResurrectIdentity(t *testing.T) {
	for _, old := range []Status{StatusActive, StatusDisabled, StatusDeleted, Status("unknown")} {
		for _, next := range []Status{StatusActive, StatusDisabled, StatusDeleted, Status("unknown")} {
			want := (old == StatusActive && next != "unknown") ||
				(old == StatusDisabled && (next == StatusDisabled || next == StatusDeleted)) ||
				(old == StatusDeleted && next == StatusDeleted)
			if old.CanTransitionTo(next) != want {
				t.Fatalf("transition %s -> %s", old, next)
			}
		}
	}
}

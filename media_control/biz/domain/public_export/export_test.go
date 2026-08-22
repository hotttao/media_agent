package public_export

import "testing"

func TestTransitions(t *testing.T) {
	if !CanTransition(StatusPending, StatusReady) || CanTransition(StatusReady, StatusFailed) || CanTransition(StatusFailed, StatusReady) {
		t.Fatal("unexpected export transitions")
	}
	if _, _, _, e := Validate("x", "y", "https://example.invalid/file"); e == nil {
		t.Fatal("expected relative path validation")
	}
}

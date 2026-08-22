package agent_task

import "testing"

func TestValidateInputObject(t *testing.T) {
	if _, _, _, _, _, err := Validate("w", "manual", "source", "key", `[]`); err == nil {
		t.Fatal("expected object validation error")
	}
	_, _, _, _, input, err := Validate("w", "manual", "source", "key", "")
	if err != nil || input != "{}" {
		t.Fatalf("unexpected default input: %q %v", input, err)
	}
}
func TestTerminal(t *testing.T) {
	if !IsTerminal(StatusCompleted) || IsTerminal(StatusWaitingTurn) {
		t.Fatal("unexpected terminal classification")
	}
}

package job

import "testing"

func TestGenerationResultIdentityTracksAcceptedCapabilityNotDelivery(t *testing.T) {
	first, err := GenerationResultKey("execution", "step", "successful-capability")
	if err != nil || len(first) != 64 {
		t.Fatal("invalid identity", err)
	}
	replay, err := GenerationResultKey("execution", "step", "successful-capability")
	if err != nil || replay != first {
		t.Fatal("same result changed identity")
	}
	for _, fields := range [][3]string{{"new-execution", "step", "successful-capability"}, {"execution", "other-step", "successful-capability"}, {"execution", "step", "regenerated-capability"}} {
		next, err := GenerationResultKey(fields[0], fields[1], fields[2])
		if err != nil || next == first {
			t.Fatal("different generation collided")
		}
	}
	for _, fields := range [][3]string{{"", "step", "task"}, {"engine", "", "task"}, {"engine", "step", ""}, {"engine", "step", " task"}} {
		if key, err := GenerationResultKey(fields[0], fields[1], fields[2]); err == nil || key != "" {
			t.Fatal("invalid identity accepted")
		}
	}
}

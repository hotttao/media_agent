package teamwork_runtime

import "testing"

func TestAllowedTopologyVersion(t *testing.T) {
	value := Version{TopologyID: "topology-1", WorkspaceID: "workspace-1", Version: 3, Status: "active"}
	if !value.Allows("workspace-1", 3) || value.Allows("workspace-1", 2) || value.Allows("workspace-1", 4) || value.Allows("workspace-2", 3) {
		t.Fatal("active version admission is incorrect")
	}
	value.Status = "deleted"
	if value.Allows("workspace-1", 3) {
		t.Fatal("deleted topology admitted a task")
	}
}

func TestValidateEvent(t *testing.T) {
	if _, err := ValidateEvent("event-1", "workspace-1", "topology-1", 2, "active"); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"", "running"} {
		if _, err := ValidateEvent("event-1", "workspace-1", "topology-1", 2, status); err == nil {
			t.Fatalf("status %q accepted", status)
		}
	}
}

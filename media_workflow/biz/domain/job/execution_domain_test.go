package job

import "testing"

func TestExecutionDomainForUserIsStableAndOpaque(t *testing.T) {
	first := ExecutionDomainForUser("user-123")
	if first != ExecutionDomainForUser("user-123") {
		t.Fatalf("execution domain is not stable: %q", first)
	}
	if first == ExecutionDomainForUser("user-456") {
		t.Fatal("different users must not share an execution domain")
	}
	if first == "" || first == "user-123" {
		t.Fatalf("execution domain must be non-empty and opaque: %q", first)
	}
}

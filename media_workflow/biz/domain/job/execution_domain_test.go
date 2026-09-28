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

func TestExecutionDomainForManagedPoolIsStableAndSeparated(t *testing.T) {
	first := ExecutionDomainForManagedPool("gpu-cn")
	if first != ExecutionDomainForManagedPool("gpu-cn") {
		t.Fatalf("managed execution domain is not stable: %q", first)
	}
	if first == ExecutionDomainForManagedPool("gpu-us") {
		t.Fatal("different managed pools must not share an execution domain")
	}
	if first == ExecutionDomainForUser("gpu-cn") || first == "gpu-cn" {
		t.Fatalf("managed domain must be isolated and opaque: %q", first)
	}
}

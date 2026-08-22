package policy

import "testing"

func TestAllowsRuntimeContext(t *testing.T) {
	if !AllowsRuntimeContext("service-token", "service-token") {
		t.Fatal("matching service token rejected")
	}
	for _, provided := range []string{"", "wrong"} {
		if AllowsRuntimeContext(provided, "service-token") {
			t.Fatalf("service token %q accepted", provided)
		}
	}
}

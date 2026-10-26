package correlation

import (
	"context"
	"testing"
)

func TestInvalidTaskCannotInheritAnotherTaskTrace(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"
	ctx := WithParent(context.Background(), parent)
	if Parent(ctx) != parent || TraceID(parent) != parent[3:35] {
		t.Fatal("valid correlation lost")
	}
	for _, invalid := range []string{"", "private\r\nAuthorization:secret", "00-00000000000000000000000000000000-1111111111111111-00"} {
		if Parent(WithParent(ctx, invalid)) != "" {
			t.Fatal("task inherited another task trace")
		}
	}
}

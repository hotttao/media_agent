package taskref

import (
	"fmt"
	"strings"
	"testing"
)

func TestReferenceRoundTripAndReservedDelimiter(t *testing.T) {
	for _, key := range []string{"model_image", "a__b", "a-b", "a_b", "人物/全身"} {
		base := Reference(key, "invoke")
		if strings.Contains(base, "__") {
			t.Fatal("business key leaked the reserved loop delimiter")
		}
		for _, iteration := range []int{0, 1, 12} {
			ref := base
			if iteration > 0 {
				ref += fmt.Sprintf("__%d", iteration)
			}
			parsed, phase, round, ok := Parse(ref)
			if !ok || parsed != key || phase != "invoke" || round != iteration {
				t.Fatalf("round trip failed: %s", ref)
			}
		}
	}
	if Reference("a-b", "invoke") == Reference("a_b", "invoke") {
		t.Fatal("different business keys collided")
	}
}

func TestParseRejectsMalformedReferences(t *testing.T) {
	for _, ref := range []string{"step__image__invoke", "step_zz_invoke", "step__invoke", "step_61_unknown", "step_61_invoke__0", "step_61_invoke__-1", "step_61_invoke__1__2", "step_61_invoke__NaN"} {
		if _, _, _, ok := Parse(ref); ok {
			t.Fatalf("malformed reference accepted: %s", ref)
		}
	}
}

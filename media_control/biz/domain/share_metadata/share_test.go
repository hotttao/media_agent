package share_metadata

import "testing"

func TestValidateResource(t *testing.T) {
	if _, _, e := Validate("", ""); e == nil {
		t.Fatal("expected required resource error")
	}
	if rt, rid, e := Validate("prompt", "p-1"); e != nil || rt != "prompt" || rid != "p-1" {
		t.Fatalf("unexpected validation: %q %q %v", rt, rid, e)
	}
}

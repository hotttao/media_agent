package workspace_metadata

import "testing"

func TestValidateTag(t *testing.T) {
	if _, _, err := Validate("", ""); err == nil {
		t.Fatal("expected required name error")
	}
	name, color, err := Validate("  urgent ", " #ff0000 ")
	if err != nil || name != "urgent" || color != "#ff0000" {
		t.Fatalf("unexpected tag validation: %q %q %v", name, color, err)
	}
}

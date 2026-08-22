package afs_binding

import "testing"

func TestValidate(t *testing.T) {
	if _, _, e := Validate("", ""); e == nil {
		t.Fatal("expected external ref error")
	}
	if _, _, _, e := ValidateMember("u", "group", ""); e == nil {
		t.Fatal("expected subject type error")
	}
}

package environment_grant

import "testing"

func TestValidate(t *testing.T) {
	if _, _, _, e := Validate("u", "group", "use"); e == nil {
		t.Fatal("expected subject type error")
	}
	if s, typ, p, e := Validate("u", "user", "admin"); e != nil || s != "u" || typ != "user" || p != "admin" {
		t.Fatalf("unexpected validation: %v", e)
	}
}

package platform_administration

import (
	"errors"
	"testing"
)

func TestValidateSettingAndAdmin(t *testing.T) {
	key, value, err := Validate("runtime.default-capacity", ` { "cpu": 2 } `)
	if err != nil || key != "runtime.default-capacity" || value != `{"cpu":2}` {
		t.Fatalf("key=%q value=%q err=%v", key, value, err)
	}
	for _, invalid := range []string{"", "UPPER", ".prefix", "has space"} {
		if _, _, err := Validate(invalid, `{}`); err == nil {
			t.Fatalf("accepted invalid key %q", invalid)
		}
	}
	if _, _, err := Validate("valid", `{`); err == nil {
		t.Fatal("accepted invalid JSON")
	}
	if err := (Actor{UserID: "admin-1", Role: "admin"}).ValidateAdmin(); err != nil {
		t.Fatal(err)
	}
	if err := (Actor{UserID: "user-1", Role: "user"}).ValidateAdmin(); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ordinary user accepted: %v", err)
	}
}

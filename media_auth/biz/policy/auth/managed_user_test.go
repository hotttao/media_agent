package auth

import (
	d "media_agent/media_auth/biz/domain/auth"
	"testing"
)

func TestManagedUserNeverBecomesInteractive(t *testing.T) {
	u := d.User{Role: "user", Status: "active", ManagedBy: "reviewdeck", PasswordHash: "even-if-corrupt"}
	if CanLogin(u) || CanChangePassword(u) {
		t.Fatal("managed account escaped its identity boundary")
	}
	if u.AuthSource(nil) != "application" {
		t.Fatal("managed account reported an interactive source")
	}
}

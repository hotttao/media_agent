package middleware

import "testing"

func TestInternalUserRequiresTokenAndActingIdentity(t *testing.T) {
	user, ok := internalUser("service-token", "service-token", " user-1 ")
	if !ok || user != "user-1" {
		t.Fatalf("user=%q ok=%v", user, ok)
	}
	for _, test := range []struct{ provided, configured, user string }{
		{"wrong", "service-token", "user-1"},
		{"", "service-token", "user-1"},
		{"service-token", "", "user-1"},
		{"service-token", "service-token", ""},
	} {
		if _, ok := internalUser(test.provided, test.configured, test.user); ok {
			t.Fatalf("accepted invalid internal identity: %+v", test)
		}
	}
}

func TestNormalizeRoleDefaultsToUser(t *testing.T) {
	if normalizeRole(" admin ") != "admin" || normalizeRole("system") != "system" {
		t.Fatal("admin roles were not preserved")
	}
	for _, role := range []string{"", "owner", "ADMIN"} {
		if normalizeRole(role) != "user" {
			t.Fatalf("role %q did not default to user", role)
		}
	}
}

func TestRuntimeContextRequiresInternalPrincipal(t *testing.T) {
	if !isInternalPrincipal(Principal{UserID: "user-1", Internal: true}) {
		t.Fatal("valid internal acting identity was rejected")
	}
	for _, value := range []Principal{{UserID: "user-1"}, {Internal: true}, {UserID: " ", Internal: true}} {
		if isInternalPrincipal(value) {
			t.Fatalf("invalid internal principal accepted: %+v", value)
		}
	}
}

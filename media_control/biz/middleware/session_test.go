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

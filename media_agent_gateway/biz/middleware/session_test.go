package middleware

import "testing"

func TestMatchesInternalToken(t *testing.T) {
	if !matchesInternalToken("service-token", "service-token") {
		t.Fatal("matching internal token was rejected")
	}
	for _, value := range [][2]string{{"", "service-token"}, {"service-token", ""}, {"wrong", "service-token"}} {
		if matchesInternalToken(value[0], value[1]) {
			t.Fatalf("invalid token accepted: %q %q", value[0], value[1])
		}
	}
}

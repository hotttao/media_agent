package application

import (
	"strings"
	"testing"
)

func TestApplicationNamespaceAndInputBoundaries(t *testing.T) {
	for _, id := range []string{"", "app:other", "../admin", "two apps", strings.Repeat("a", 65)} {
		if ValidID(id) {
			t.Fatalf("accepted invalid application ID")
		}
	}
	for _, id := range []string{"reviewdeck", "media-tool_2"} {
		if !ValidID(id) {
			t.Fatal("valid slug rejected")
		}
	}
	for _, external := range []string{"", " ", "a/b", "a\\b", "a\x00b", strings.Repeat("x", 257)} {
		if ValidProfile(external, "Name", "") {
			t.Fatal("invalid external identity accepted")
		}
	}
}

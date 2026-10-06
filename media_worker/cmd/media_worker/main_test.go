package main

import (
	"testing"

	"media_agent/media_worker/internal/gateway"
)

func TestSelectCapabilities(t *testing.T) {
	available := []gateway.Capability{
		{Key: "media.image.model", Version: 1, Concurrency: 1},
		{Key: "media.video.generate", Version: 1, Concurrency: 2},
	}
	selected, err := selectCapabilities(available, "media.video.generate@1,media.video.generate@1")
	if err != nil {
		t.Fatalf("selectCapabilities() error = %v", err)
	}
	if len(selected) != 1 || selected[0].Key != "media.video.generate" || selected[0].Concurrency != 2 {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestSelectCapabilitiesRejectsUnknownVersion(t *testing.T) {
	_, err := selectCapabilities([]gateway.Capability{{Key: "media.image.model", Version: 1}}, "media.image.model@2")
	if err == nil {
		t.Fatal("selectCapabilities() should reject an unsupported capability version")
	}
}

func TestParseMultipleFixtureResultsDoesNotExposeConfiguration(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `["https://storage.example/a"]`, `[42,true]`, `secret-invalid-json`} {
		if _, err := parseFixtureResultURLs(raw); err == nil || err.Error() == raw {
			t.Fatal("malformed candidate configuration accepted or leaked")
		}
	}
	urls, err := parseFixtureResultURLs(`["https://storage.example/a","https://storage.example/b"]`)
	if err != nil || len(urls) != 2 {
		t.Fatal("valid configuration rejected", err)
	}
}

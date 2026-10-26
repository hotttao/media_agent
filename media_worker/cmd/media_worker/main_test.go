package main

import "testing"

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

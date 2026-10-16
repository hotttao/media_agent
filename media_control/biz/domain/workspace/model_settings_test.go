package workspace

import "testing"

func TestModelSettingsCompatibilityAndCredentialFreeURL(t *testing.T) {
	for _, v := range []struct {
		agent, protocol, name, url string
		ok                         bool
	}{
		{"claude-code", "anthropic", "deepseek/model", "https://api.example.test/", true},
		{"goose", "openai", "deepseek/model", "https://api.example.test/v1", true},
		{"goose", "anthropic", "model", "", true},
		{"codex", "", "", "", true},
		{"claude-code", "openai", "model", "", false},
		{"goose", "unknown", "model", "", false},
		{"goose", "openai", "model", "https://user:secret@api.example.test", false},
		{"goose", "openai", "model", "https://api.example.test?api_key=secret", false},
		{"goose", "openai", "model", "file:///secret", false},
		{"goose", "openai", "model\nmalformed", "", false},
		{"codex", "openai", "model", "", false},
	} {
		_, _, _, e := NormalizeModelSettings(v.agent, v.protocol, v.name, v.url)
		if (e == nil) != v.ok {
			t.Fatalf("agent=%s protocol=%s validity mismatch", v.agent, v.protocol)
		}
	}
}

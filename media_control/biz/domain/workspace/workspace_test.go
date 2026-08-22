package workspace

import "testing"

func TestParseAgentType(t *testing.T) {
	for _, input := range []string{"codex", "CODEX", " claude-code "} {
		if _, err := ParseAgentType(input); err != nil {
			t.Fatalf("ParseAgentType(%q): %v", input, err)
		}
	}
	if _, err := ParseAgentType("goose"); err == nil {
		t.Fatal("unsupported Agent Type accepted")
	}
}

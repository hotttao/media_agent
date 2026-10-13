package sessioncredential

import (
	"bytes"
	"testing"
)

func TestPersistentKeysRotationAndScope(t *testing.T) {
	old := bytes.Repeat([]byte{1}, 32)
	first, err := New("a", map[string][]byte{"a": old})
	if err != nil {
		t.Fatal(err)
	}
	token, err := first.Derive("st1", "a", "session", "user", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	second, err := New("b", map[string][]byte{"a": old, "b": bytes.Repeat([]byte{2}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := second.Derive("st1", "a", "session", "user", "workspace")
	if err != nil || restored != token {
		t.Fatal("rotation changed a pinned credential")
	}
	for _, scope := range [][]string{{"session", "other", "workspace"}, {"other", "user", "workspace"}, {"session", "user", "other"}} {
		changed, err := second.Derive("st1", "a", scope...)
		if err != nil || Matches(changed, Hash(token)) {
			t.Fatal("credential escaped its scope")
		}
	}
	instance, err := second.Derive("it1", "a", "session", "user", "workspace")
	if err != nil || instance == token {
		t.Fatal("credential purposes overlap")
	}
	removed, _ := New("b", map[string][]byte{"b": bytes.Repeat([]byte{2}, 32)})
	if _, err = removed.Derive("st1", "a", "session", "user", "workspace"); err == nil {
		t.Fatal("retired key accepted")
	}
}

package runtimeauth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRuntimeAssertionWireContract(t *testing.T) {
	signer, err := New(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize)))
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{WorkspaceID: "wire-workspace", AgentConfigID: "wire-config", ReplicaID: "0", IncarnationID: "wire-boot", ExecutionEpoch: 3}
	path := "/api/v1/runtime-sessions/wire:quiesce"
	body := []byte(`{"transfer_id":"wire-transfer"}`)
	token, err := signer.Sign(scope, "POST", path, body)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range signer.PublicKeys() {
		public, err := base64.RawURLEncoding.DecodeString(text)
		if err != nil {
			t.Fatal(err)
		}
		if !ed25519.Verify(public, []byte(parts[0]), signature) {
			t.Fatal("signature cannot be verified with published key")
		}
		if ed25519.Verify(public, []byte(parts[0]+"x"), signature) {
			t.Fatal("altered claims accepted")
		}
	}
	if output := os.Getenv("RUNTIME_AUTH_TEST_FIXTURE"); output != "" {
		value := map[string]any{"keys": signer.PublicKeys(), "token": token, "method": "POST", "path": path, "body": string(body)}
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

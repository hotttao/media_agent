package teamwork

import "testing"

func TestValidateTopologyReferences(t *testing.T) {
	members := `[{"member_id":"writer","agent_config_id":"cfg-1","name":"Writer"},{"member_id":"reviewer","agent_config_id":"cfg-2","name":"Reviewer"}]`
	if _, _, _, _, err := Validate("topology", "missing", members, `[]`); err == nil {
		t.Fatal("expected missing entry error")
	}
	_, _, _, _, err := Validate("topology", "writer", members, `[{"from_member_id":"writer","to_member_id":"missing","mode":"delegate"}]`)
	if err == nil {
		t.Fatal("expected unknown edge endpoint error")
	}
	_, _, ms, es, err := Validate("topology", "writer", members, `[{"from_member_id":"writer","to_member_id":"reviewer","mode":"delegate"}]`)
	if err != nil || len(ms) != 2 || len(es) != 1 {
		t.Fatalf("unexpected valid topology: %v", err)
	}
}

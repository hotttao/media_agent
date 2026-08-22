package runtime_environment

import "testing"

func TestValidateCapacity(t *testing.T) {
	if _, _, _, err := Validate("env", "http://runtime", `[]`); err == nil {
		t.Fatal("expected object validation error")
	}
	name, endpoint, capacity, err := Validate(" env ", " http://runtime ", `{"cpu":4}`)
	if err != nil || name != "env" || endpoint != "http://runtime" || capacity != `{"cpu":4}` {
		t.Fatalf("unexpected validation: %q %q %q %v", name, endpoint, capacity, err)
	}
}

func TestParseStatus(t *testing.T) {
	if _, err := ParseStatus("unknown"); err == nil {
		t.Fatal("expected invalid status")
	}
}

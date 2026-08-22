package batch_run

import "testing"

func TestValidateFreezesUniqueJSONObjectItems(t *testing.T) {
	workspace, key, items, err := Validate(" workspace ", " key ", []Item{{ItemKey: "one", SessionID: "session", InputJSON: `{"message":"hi"}`}})
	if err != nil || workspace != "workspace" || key != "key" || len(items) != 1 {
		t.Fatalf("workspace=%q key=%q items=%+v error=%v", workspace, key, items, err)
	}
	if _, _, _, err := Validate("workspace", "key", []Item{{ItemKey: "one", SessionID: "session"}, {ItemKey: "one", SessionID: "session"}}); err == nil {
		t.Fatal("duplicate item key was accepted")
	}
	if _, _, _, err := Validate("workspace", "key", []Item{{ItemKey: "one", SessionID: "session", InputJSON: `[]`}}); err == nil {
		t.Fatal("array input was accepted")
	}
}

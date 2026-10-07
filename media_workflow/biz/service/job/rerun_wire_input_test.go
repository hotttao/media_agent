package job

import (
	"bytes"
	"encoding/json"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

func TestRerunConfirmationAcceptsTypedInputAfterActualJSONRoundTrip(t *testing.T) {
	expected := map[string]any{"job_id": "job", "inputs": map[string]any{"product": map[string]any{"value": &domain.ProductData{ID: "product", Name: "changed", ImageURL: "object_path:materials/org/image.png"}}, "virtual_ip": map[string]any{"value": &domain.VirtualIpData{ID: "person", Name: "person"}}, "large": json.Number("9007199254740993")}}
	wire, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(wire))
	decoder.UseNumber()
	var actual map[string]any
	if decoder.Decode(&actual) != nil {
		t.Fatal("wire input decode failed")
	}
	oldActual, _ := json.Marshal(actual)
	if bytes.Equal(oldActual, wire) {
		t.Fatal("fixture did not reproduce old struct/map ordering mismatch")
	}
	before := &domain.EngineExecution{WorkflowInstanceID: "execution", DefinitionName: "frozen", DefinitionVersion: 1, Status: "FAILED", Input: map[string]any{"old": true}, Tasks: []domain.EngineTask{{TaskID: "old"}}}
	after := &domain.EngineExecution{WorkflowInstanceID: "execution", DefinitionName: "frozen", DefinitionVersion: 1, Status: "RUNNING", Input: actual, Tasks: []domain.EngineTask{{TaskID: "new"}}}
	if !sameRerunInput(actual, expected) || !confirmedRerun(before, after, expected, false) || !confirmedRerun(before, after, expected, true) {
		t.Fatal("typed input and identical decoded native input compared unequal")
	}
	actual["inputs"].(map[string]any)["large"] = json.Number("9007199254740992")
	if sameRerunInput(actual, expected) {
		t.Fatal("canonicalization lost large integer precision")
	}
}

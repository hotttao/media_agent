package job

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExecutionStartFreezesSemanticPayload(t *testing.T) {
	s := ExecutionStart{ID: "start", JobID: "job", WorkflowName: "compiled", WorkflowVersion: 1, CompilationHash: strings.Repeat("a", 64), CorrelationID: "job", WorkflowInput: json.RawMessage(`{"job_id":"job","inputs":{}}`), TaskDomains: json.RawMessage(`{"capability":"private-domain"}`)}
	s.PayloadDigest, _ = s.Digest()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"correlation", "input", "domains", "version", "hash", "digest"} {
		changed := s
		switch field {
		case "correlation":
			changed.CorrelationID = "other"
		case "input":
			changed.WorkflowInput = json.RawMessage(`{"job_id":"other"}`)
		case "domains":
			changed.TaskDomains = json.RawMessage(`{"capability":"different"}`)
		case "version":
			changed.WorkflowVersion = 2
		case "hash":
			changed.CompilationHash = strings.Repeat("B", 64)
		case "digest":
			changed.PayloadDigest = "different"
		}
		if changed.Validate() == nil {
			t.Fatal("mutated frozen start accepted: " + field)
		}
	}
	pretty := s
	pretty.WorkflowInput = json.RawMessage(`{ "inputs": {}, "job_id": "job" }`)
	if digest, _ := pretty.Digest(); digest != s.PayloadDigest {
		t.Fatal("digest depends on JSON whitespace or object order")
	}
}

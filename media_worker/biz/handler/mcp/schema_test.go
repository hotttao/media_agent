package mcp

import (
	"media_agent/hertz_infra/mcpprotocol"
	policy "media_agent/media_worker/biz/policy/media_management"
	flow "media_agent/media_worker/biz/workflow/media_management"
	"testing"
)

func TestAllBatchSchemasCompile(t *testing.T) {
	if _, err := New(flow.Management{}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range policy.Names {
		t.Run(name, func(t *testing.T) {
			v, err := mcpprotocol.CompileSchema(inputSchema(name))
			if err != nil {
				t.Fatal(err)
			}
			for _, raw := range []string{`{}`, `{"items":[]}`, `{"items":[{"item_id":"x","params":{},"extra":true}]}`} {
				if err := v.Validate([]byte(raw)); err == nil {
					t.Fatalf("accepted invalid batch %s", raw)
				}
			}
		})
	}
}

func TestJobSubmitSchemaRequiresBatchAndIndependentKeys(t *testing.T) {
	v, err := mcpprotocol.CompileSchema(inputSchema("jobs_submit"))
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"items":[{"item_id":"one","params":{"workflow_id":"w","workflow_version":1,"idempotency_key":"key-one","input":{}}},{"item_id":"two","params":{"workflow_id":"w","workflow_version":1,"idempotency_key":"key-two","input":{}}}]}`
	if err := v.Validate([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"workflow_id":"w"}`, `{"items":[{"item_id":"one","params":{"workflow_id":"w","workflow_version":1,"input":{}}}]}`} {
		if err := v.Validate([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid submission %s", raw)
		}
	}
}

func TestSubmissionOutputRequiresJobReceiptForEverySuccess(t *testing.T) {
	v, err := mcpprotocol.CompileSchema(outputSchema("jobs_submit"))
	if err != nil {
		t.Fatal(err)
	}
	const prefix = `{"summary":{"total":1,"succeeded":1,"failed":0,"unknown":0,"not_dispatched":0},"results":[`
	const suffix = `]}`
	for _, row := range []string{
		`{"item_id":"a","status":"succeeded","data":{"job_id":"job-a","status":"PENDING"}}`,
		`{"item_id":"a","status":"succeeded","data":{"job_id":"job-a","status":1}}`,
	} {
		if err := v.Validate([]byte(prefix + row + suffix)); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []string{
		`{"item_id":"a","status":"succeeded","data":{"status":"PENDING"}}`,
		`{"item_id":"a","status":"succeeded","data":{"job_id":"job-a"}}`,
		`{"item_id":"a","status":"succeeded","error":{"code":"failure","message":"failed"}}`,
		`{"item_id":"a","status":"failed","data":{"job_id":"job-a","status":1}}`,
	} {
		if err := v.Validate([]byte(prefix + row + suffix)); err == nil {
			t.Fatalf("accepted malformed receipt: %s", row)
		}
	}
}

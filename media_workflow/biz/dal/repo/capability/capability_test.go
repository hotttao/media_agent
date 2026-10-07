package capability

import (
	"testing"

	"media_agent/media_workflow/biz/dal/model"
)

func contractRow() *model.ExecutionCapability {
	output, method, kind := "IMAGE", "i2i", "first_frame_prompt"
	return &model.ExecutionCapability{CapabilityKey: "jimeng.i2i.first_frame_prompt", Version: 1, Provider: "jimeng", Mode: "image_generation", GenerationMethod: &method, GenerationType: &kind, OutputMediaType: &output, ExecutionTarget: "personal_worker", ConductorTaskDef: "jimeng_i2i_first_frame_v1", InputSchema: []byte(`[{"name":"prompt","type":"string","required":true},{"name":"images","type":"resource_ref_list","required":true,"media_type":"IMAGE","min_items":1,"max_items":1}]`), RetryPolicy: []byte(`{"retry_count":3}`), Status: "published"}
}

func TestRegistryProjectionPreservesContractFields(t *testing.T) {
	c, err := toDomain(contractRow())
	if err != nil || c.Ref() != "jimeng.i2i.first_frame_prompt@1" || c.GenerationMethod != "i2i" || c.GenerationType != "first_frame_prompt" || c.OutputMediaType != "IMAGE" || !c.Inputs[1].AcceptsResourceCount(1) || c.Inputs[1].AcceptsResourceCount(2) {
		t.Fatal("registry lost fixed worker signature")
	}
}

func TestRegistryRejectsPoisonedPublishedSignature(t *testing.T) {
	for _, mutation := range []string{"missing_dimension", "mismatched_key", "bad_images", "business_output", "malformed_json"} {
		t.Run(mutation, func(t *testing.T) {
			row := contractRow()
			switch mutation {
			case "missing_dimension":
				row.GenerationType = nil
			case "mismatched_key":
				row.CapabilityKey = "media.image.model"
			case "bad_images":
				row.InputSchema = []byte(`[{"name":"prompt","type":"string","required":true},{"name":"images","type":"resource_ref_list","required":true,"media_type":"VIDEO","min_items":1,"max_items":1}]`)
			case "business_output":
				row.OutputAssetKind = "model_image"
			case "malformed_json":
				row.InputSchema = []byte(`{`)
			}
			if _, err := toDomain(row); err == nil {
				t.Fatal("poisoned provider record exposed")
			}
		})
	}
}

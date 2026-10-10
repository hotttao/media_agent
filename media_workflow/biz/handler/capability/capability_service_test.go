package capability

import (
	"encoding/json"
	domain "media_agent/media_workflow/biz/domain/capability"
	"strings"
	"testing"
)

func TestPublicCapabilityProjectionOmitsPrivateEngineRouting(t *testing.T) {
	row := &domain.Capability{Key: "jimeng.i2i.multi_image_prompt.managed", Version: 2, ConductorTaskDef: "private-task-def", ManagedPoolKey: "private-pool", Provider: "jimeng", ExecutionTarget: domain.ExecutionTargetManagedWorker, Inputs: []domain.Input{{Name: "images", Type: domain.InputResourceRefList, MediaType: domain.MediaImage, MinItems: 1}}}
	data, err := json.Marshal(toView(row))
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-task-def", "private-pool", "conductor_task_def", "managed_pool_key"} {
		if strings.Contains(string(data), private) {
			t.Fatal("private routing exposed")
		}
	}
	if !strings.Contains(string(data), "resource_ref_list") || !strings.Contains(string(data), "IMAGE") {
		t.Fatal("input constraints omitted")
	}
}

package capability

import "testing"

func TestDownloadContractIsAcquisitionAndClientOnly(t *testing.T) {
	c := Capability{Key: "wx_channel.download.record", Version: 1, Provider: "wx_channel", Mode: "reference_import", GenerationMethod: Download, GenerationType: Record, OutputMediaType: MediaVideo, ConductorTaskDef: "wx_channel_download_record_v1", ExecutionTarget: ExecutionTargetPersonalWorker, Inputs: []Input{{Name: "record_id", Type: InputString, Required: true}}}
	if err := c.ValidateContract(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Capability){func(c *Capability) { c.Mode = "video_generation" }, func(c *Capability) {
		c.ExecutionTarget = ExecutionTargetManagedWorker
		c.ManagedPoolKey = "pool"
		c.Key += ".managed"
	}, func(c *Capability) { c.Inputs = []Input{{Name: "url", Type: InputString, Required: true}} }} {
		bad := c
		change(&bad)
		if bad.ValidateContract() == nil {
			t.Fatal("invalid download contract accepted", bad)
		}
	}
}

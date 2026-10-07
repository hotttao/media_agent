package main

import "testing"

func TestExplicitProviderSamplePreservesOldTemplatesAndMapsImages(t *testing.T) {
	legacy := []templateSpec{{Key: "model", Name: "old model", CapabilityKey: "media.image.model", Invocation: map[string]any{"prompt": "render", "resources": "person+product"}}, {Key: "style", Name: "old style", Invocation: map[string]any{"prompt": "render", "resources": "model+pose"}}, {Key: "first_frame", Name: "old frame", Invocation: map[string]any{"prompt": "render", "resources": "style+scene"}}, {Key: "video", Name: "old video", Invocation: map[string]any{"prompt": "render", "resources": "first_frame"}}}
	got, err := providerFixtureTemplates(legacy, "jimeng.i2i.multi_image_prompt@1", "doubao.i2v.first_frame_prompt@1")
	if err != nil || len(got) != 4 {
		t.Fatal("explicit sample selection failed", err)
	}
	for i, spec := range got {
		if spec.CapabilityVersion != 1 || spec.Name == legacy[i].Name || spec.Invocation["resources"] != nil || spec.Invocation["images"] != legacy[i].Invocation["resources"] {
			t.Fatal("provider mapping changed old templates or lost image order")
		}
		if i < 3 && (spec.CapabilityKey != "jimeng.i2i.multi_image_prompt" || spec.Platform != "jimeng" || spec.GenerationMethod != "i2i") {
			t.Fatal("business image steps do not reuse image capability")
		}
	}
	if got[3].CapabilityKey != "doubao.i2v.first_frame_prompt" || got[3].Platform != "doubao" || legacy[0].Name != "old model" || legacy[0].Invocation["images"] != nil {
		t.Fatal("video mapping or old snapshot changed")
	}
	for _, refs := range [][2]string{{"", "doubao.i2v.first_frame_prompt@1"}, {"media.image.model@1", "doubao.i2v.first_frame_prompt@1"}, {"jimeng.i2i.first_frame_prompt@1", "doubao.i2v.first_frame_prompt@1"}, {"jimeng.i2i.multi_image_prompt@1", "jimeng.i2i.multi_image_prompt@1"}, {"jimeng.i2i.multi_image_prompt.managed@1", "doubao.i2v.first_frame_prompt@1"}, {"jimeng.i2i.multi_image_prompt@0", "doubao.i2v.first_frame_prompt@1"}} {
		if _, err := providerFixtureTemplates(legacy, refs[0], refs[1]); err == nil {
			t.Fatal("incompatible explicit capability accepted")
		}
	}
}

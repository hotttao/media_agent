package main

import (
	"testing"
	"time"
)

func TestReconciliationUsesSourcePrefixRatherThanReportAssertions(t *testing.T) {
	created := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	m := manifest{ModelImages: []sourceImage{{ID: "model", CreatedAt: created}}, StyleImages: []sourceImage{{ID: "style", CreatedAt: created.Add(time.Hour)}}, Groups: []lineageGroup{{ID: "style:style", ExecutionState: "awaiting_resume", ModelImageID: "model", StyleImageID: "style"}}}
	got, err := sourceHistoryArtifacts(&m, "incomplete_group", "style:style")
	if err != nil || len(got) != 2 || got["model_image"].ID != "model" || !got["style_image"].CreatedAt.Equal(created.Add(time.Hour)) {
		t.Fatalf("source prefix mismatch: %+v %v", got, err)
	}
	m.StyleImages = nil
	if _, err := sourceHistoryArtifacts(&m, "incomplete_group", "style:style"); err == nil {
		t.Fatal("missing source prefix was accepted")
	}
	if _, err := sourceHistoryArtifacts(&m, "incomplete_group", "style:unknown"); err == nil {
		t.Fatal("unknown source was accepted")
	}
}

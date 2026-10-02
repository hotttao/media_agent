package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClassifyLegacyTasks(t *testing.T) {
	for _, tc := range []struct {
		input, want string
	}{
		{`{"source":"manual_upload"}`, "manual_upload"},
		{`{"source":"workflow_cli"}`, "workflow_cli"},
		{`{}`, "unknown"},
		{`broken`, "unknown"},
	} {
		if got := classify(json.RawMessage(tc.input)); got != tc.want {
			t.Errorf("classify(%q)=%q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMySQLDSN(t *testing.T) {
	dsn, err := mysqlDSN("mysql://user:p%40ss@localhost:3306/media_ai")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "user:p@ss@tcp(localhost:3306)/media_ai?") || !strings.Contains(dsn, "parseTime=true") {
		t.Fatalf("unexpected DSN shape: %q", strings.ReplaceAll(dsn, "p@ss", "<redacted>"))
	}
	if _, err := mysqlDSN("mysql://localhost/media_ai"); err == nil {
		t.Fatal("missing user accepted")
	}
}

func TestBuildLineagesFollowsRelationshipsAndFlagsBrokenReferences(t *testing.T) {
	m := manifest{
		Templates:   []workflowTemplate{{ID: "tpl", VirtualIPID: "ip", PoseID: "pose", SceneID: "scene", MovementIDs: []string{"move"}}},
		ModelImages: []sourceImage{{ID: "model", ProductID: "product", VirtualIPID: "ip"}},
		StyleImages: []sourceImage{{ID: "style", ParentID: "model", PoseID: "pose"}},
		FirstFrames: []sourceImage{{ID: "frame", ParentID: "style", SceneID: "scene"}},
		Tasks:       []videoTask{{ID: "task", Videos: []video{{ID: "video", TaskID: "task", ProductID: "product", IPID: "ip", ModelImageID: "model", StyleImageID: "style", FirstFrameID: "frame", MovementID: "move"}}}},
		VideoPushes: []sourceVideoPush{{ID: "push", VideoIDs: []string{"video", "missing"}}},
	}
	buildLineages(&m, "")
	if len(m.Lineages) != 1 {
		t.Fatalf("lineages=%d", len(m.Lineages))
	}
	l := m.Lineages[0]
	if l.PoseID != "pose" || l.SceneID != "scene" || len(l.TemplateIDs) != 1 || l.TemplateIDs[0] != "tpl" || len(l.VideoPushIDs) != 1 || l.VideoPushIDs[0] != "push" {
		t.Fatalf("incorrect lineage: %+v", l)
	}
	if len(m.Issues) != 1 || m.Issues[0].Kind != "video_push_missing_video" {
		t.Fatalf("issues=%+v", m.Issues)
	}
}

func TestBuildGroupsExcludesBrokenRecordsButKeepsValidPartialChains(t *testing.T) {
	m := manifest{
		Lineages: []videoLineage{
			{VideoID: "a", ProductID: "product", VirtualIPID: "ip", FirstFrameID: "frame", SceneID: "scene", TemplateIDs: []string{"tpl"}},
			{VideoID: "b", ProductID: "product", VirtualIPID: "ip", FirstFrameID: "frame", SceneID: "scene", TemplateIDs: []string{"tpl"}},
			{VideoID: "broken", ProductID: "product", VirtualIPID: "ip", FirstFrameID: "other"},
		},
		VideoPushes: []sourceVideoPush{
			{ID: "good", VideoIDs: []string{"a", "b", "a"}, ProductID: "product", VirtualIPID: "ip", SceneID: "scene"},
			{ID: "missing", VideoIDs: []string{"b", "absent"}, ProductID: "product", VirtualIPID: "ip"},
			{ID: "bad", VideoIDs: []string{"broken"}, ProductID: "product", VirtualIPID: "ip"},
			{ID: "empty"},
		},
		Issues: []lineageIssue{
			{Kind: "video_missing_first_frame", SourceID: "broken"},
			{Kind: "task_without_video", SourceID: "task"},
		},
	}
	buildGroups(&m, "")
	if len(m.Groups) != 1 || m.Groups[0].ID != "push:good" || strings.Join(m.Groups[0].VideoIDs, ",") != "a,b" {
		t.Fatalf("groups=%+v", m.Groups)
	}
	if len(m.Excluded) != 5 {
		t.Fatalf("excluded=%+v", m.Excluded)
	}
	for _, exclusion := range m.Excluded {
		if exclusion.SourceID == "a" || exclusion.SourceID == "b" {
			t.Fatalf("valid video excluded: %+v", exclusion)
		}
	}

	partial := manifest{Lineages: []videoLineage{
		{VideoID: "a", ProductID: "product", VirtualIPID: "ip", FirstFrameID: "frame"},
		{VideoID: "b", ProductID: "product", VirtualIPID: "ip", FirstFrameID: "frame"},
	}}
	buildGroups(&partial, "")
	if len(partial.Groups) != 1 || partial.Groups[0].Kind != "partial_frame" || strings.Join(partial.Groups[0].VideoIDs, ",") != "a,b" {
		t.Fatalf("valid partial chain lost: %+v", partial.Groups)
	}
}

func TestSampleGroupsNeverClaimCompletePush(t *testing.T) {
	m := manifest{
		Lineages:    []videoLineage{{VideoID: "a", ProductID: "product", VirtualIPID: "ip", FirstFrameID: "frame"}},
		VideoPushes: []sourceVideoPush{{ID: "push", VideoIDs: []string{"a", "other"}}},
	}
	buildGroups(&m, "task")
	if len(m.Groups) != 1 || m.Groups[0].Kind != "sample_projection" || len(m.Excluded) != 0 {
		t.Fatalf("sample incorrectly claimed complete push: groups=%+v excluded=%+v", m.Groups, m.Excluded)
	}
}

func TestImageOnlyGroupsKeepPartialResultsAndExcludeBrokenParents(t *testing.T) {
	m := manifest{
		ModelImages: []sourceImage{{ID: "model", ProductID: "product", VirtualIPID: "ip"}, {ID: "unused", ProductID: "product", VirtualIPID: "ip"}},
		StyleImages: []sourceImage{
			{ID: "style", ParentID: "model", ProductID: "product", VirtualIPID: "ip"},
			{ID: "orphan", ParentID: "absent", ProductID: "product", VirtualIPID: "ip"},
		},
		FirstFrames: []sourceImage{
			{ID: "frame", ParentID: "style", ProductID: "product", VirtualIPID: "ip"},
			{ID: "bad-frame", ParentID: "orphan", ProductID: "product", VirtualIPID: "ip"},
		},
	}
	buildGroups(&m, "")
	if len(m.Groups) != 2 || m.Groups[0].Kind != "partial_first_frame" || m.Groups[0].ExecutionState != "awaiting_resume" || m.Groups[0].NextStepKey != "video" || m.Groups[1].Kind != "partial_model_image" || m.Groups[1].NextStepKey != "style_image" {
		t.Fatalf("partial image groups=%+v", m.Groups)
	}
	if len(m.Excluded) != 2 || m.Excluded[0].SourceID != "bad-frame" || m.Excluded[1].SourceID != "orphan" {
		t.Fatalf("broken image exclusions=%+v", m.Excluded)
	}
}

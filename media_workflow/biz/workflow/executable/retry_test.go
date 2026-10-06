package executable

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func retryDAG() ExecutableWorkflowIR {
	step := func(key string, order int, parents ...string) ExecutableStepIR {
		context := map[string]ValueRef{}
		for _, parent := range parents {
			context[parent] = ValueRef{Source: ValueStepOutput, Ref: parent}
		}
		return ExecutableStepIR{Key: key, Order: order, DependsOn: parents, Context: context}
	}
	return ExecutableWorkflowIR{SchemaVersion: SchemaVersion, WorkflowID: "frozen", WorkflowVersion: 7, OutputStepKey: "video", Steps: []ExecutableStepIR{
		step("source", 1), step("model", 2, "source"), step("background", 3, "source"), step("pose", 4, "source"), step("outfit", 5, "model", "pose"), step("join", 6, "outfit", "background"), step("video", 7, "join"),
	}}
}

func TestRetryImpactUsesTransitiveDependenciesAndPreservesSiblings(t *testing.T) {
	for _, tc := range []struct {
		target              string
		affected, preserved []string
		boundary            []RetryBoundary
	}{
		{"model", []string{"model", "outfit", "join", "video"}, []string{"source", "background", "pose"}, []RetryBoundary{{"source", "model", "source"}, {"pose", "outfit", "pose"}, {"background", "join", "background"}}},
		{"background", []string{"background", "join", "video"}, []string{"source", "model", "pose", "outfit"}, []RetryBoundary{{"source", "background", "source"}, {"outfit", "join", "outfit"}}},
		{"video", []string{"video"}, []string{"source", "model", "background", "pose", "outfit", "join"}, []RetryBoundary{{"join", "video", "join"}}},
		{"source", []string{"source", "model", "background", "pose", "outfit", "join", "video"}, []string{}, []RetryBoundary{}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			ir := retryDAG()
			before, _ := json.Marshal(ir)
			got, err := PlanStepRetry(ir, tc.target)
			want := RetryImpact{TargetStep: tc.target, Affected: tc.affected, Preserved: tc.preserved, Boundary: tc.boundary}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("impact=%+v err=%v", got, err)
			}
			after, _ := json.Marshal(ir)
			if string(before) != string(after) {
				t.Fatal("frozen IR modified")
			}
			for i, j := 0, len(ir.Steps)-1; i < j; i, j = i+1, j-1 {
				ir.Steps[i], ir.Steps[j] = ir.Steps[j], ir.Steps[i]
			}
			again, err := PlanStepRetry(ir, tc.target)
			if err != nil || !reflect.DeepEqual(again, want) {
				t.Fatal("storage array order changed impact")
			}
		})
	}
}

func TestRetryGraphFailsClosedBeforePlanning(t *testing.T) {
	for _, mutation := range []string{"schema", "identity", "version", "unknown_target", "duplicate_key", "missing_dependency", "duplicate_dependency", "self", "hidden_dependency", "undeclared_binding", "cycle", "extra_sink", "missing_output", "output_not_sink", "unknown_source", "empty_slot"} {
		t.Run(mutation, func(t *testing.T) {
			ir, target := retryDAG(), "model"
			switch mutation {
			case "schema":
				ir.SchemaVersion++
			case "identity":
				ir.WorkflowID = ""
			case "version":
				ir.WorkflowVersion = 0
			case "unknown_target":
				target = "absent"
			case "duplicate_key":
				ir.Steps[2].Key = "model"
			case "missing_dependency":
				ir.Steps[1].DependsOn = []string{"absent"}
			case "duplicate_dependency":
				ir.Steps[1].DependsOn = []string{"source", "source"}
			case "self":
				ir.Steps[1].DependsOn = []string{"model"}
			case "hidden_dependency":
				delete(ir.Steps[1].Context, "source")
			case "undeclared_binding":
				ir.Steps[1].Context["extra"] = ValueRef{Source: ValueStepOutput, Ref: "pose"}
			case "cycle":
				ir.Steps[0].DependsOn = []string{"model"}
				ir.Steps[0].Context["model"] = ValueRef{Source: ValueStepOutput, Ref: "model"}
			case "extra_sink":
				ir.Steps = append(ir.Steps, ExecutableStepIR{Key: "orphan", Order: 8})
			case "missing_output":
				ir.OutputStepKey = "absent"
			case "output_not_sink":
				ir.OutputStepKey = "model"
			case "unknown_source":
				ir.Steps[1].Context["unknown"] = ValueRef{Source: "invalid"}
			case "empty_slot":
				ir.Steps[1].Context[""] = ValueRef{Source: ValueConst}
			}
			impact, err := PlanStepRetry(ir, target)
			if !errors.Is(err, ErrInvalidRetryGraph) || !reflect.DeepEqual(impact, RetryImpact{}) {
				t.Fatal("invalid graph produced a partial recovery plan")
			}
		})
	}
}

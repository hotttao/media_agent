package client

import (
	"context"
	"errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	contract "media_agent/hertz_infra/agentworkload"
	"testing"
)

func workloadSpec() contract.Spec {
	s := contract.Spec{Driver: "kubernetes", Namespace: "agents", EnvironmentID: "env", WorkspaceID: "ws", AgentConfigID: "cfg", AgentType: "codex", Image: "image:approved", Port: 3001, Replicas: 2, CPU: "1", Memory: "2Gi", StorageSize: "10Gi", SharedSecret: "shared", GatewayURL: "http://gateway:8006"}
	n := contract.Name(s.EnvironmentID, s.WorkspaceID)
	s.ClaimName = n + "-workspace"
	s.IdentitySecret = n + "-identity"
	return s
}
func TestNativeWorkloadCASAndOwnedFields(t *testing.T) {
	ctx := context.Background()
	spec := workloadSpec()
	obj := &unstructured.Unstructured{Object: map[string]any{"apiVersion": contract.APIVersion, "kind": "AgentWorkload", "spec": spec.Object(), "status": map[string]any{"server": "retained"}}}
	obj.SetName(contract.Name(spec.EnvironmentID, spec.WorkspaceID))
	obj.SetNamespace(spec.Namespace)
	obj.SetUID(types.UID("native-uid"))
	obj.SetResourceVersion("1")
	obj.SetLabels(spec.Labels())
	obj.SetAnnotations(map[string]string{contract.TargetVersionAnnotation: "1", "operator-note": "retain"})
	obj.SetFinalizers([]string{"operator.example/retain"})
	api := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	adapter := KubernetesWorkloads{API: api, EnvironmentID: spec.EnvironmentID, Namespace: spec.Namespace}
	state, e := adapter.Read(ctx, spec)
	if e != nil {
		t.Fatal(e)
	}
	state.Operation = "scale:2"
	state.PendingReplicas = 1
	state.PendingVersion = 2
	if e = adapter.Save(ctx, state); e != nil {
		t.Fatal(e)
	}
	current, e := api.Resource(workloadGVR).Namespace(spec.Namespace).Get(ctx, obj.GetName(), metav1.GetOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if current.GetAnnotations()["operator-note"] != "retain" || len(current.GetFinalizers()) != 1 || current.Object["status"] == nil {
		t.Fatal("Gateway overwrote server/operator fields")
	}
	again, e := adapter.Read(ctx, spec)
	if e != nil || again.Operation != state.Operation || again.PendingReplicas != 1 || again.Spec.Replicas != 2 {
		t.Fatalf("lost durable drain: %+v %v", again, e)
	}
	current.SetResourceVersion("2")
	if _, e = api.Resource(workloadGVR).Namespace(spec.Namespace).Update(ctx, current, metav1.UpdateOptions{}); e != nil {
		t.Fatal(e)
	}
	if e = adapter.Save(ctx, state); !errors.Is(e, ErrWorkloadConflict) {
		t.Fatal("stale native revision accepted")
	}
	current.SetLabels(map[string]string{contract.WorkspaceLabel: "other"})
	if _, e = api.Resource(workloadGVR).Namespace(spec.Namespace).Update(ctx, current, metav1.UpdateOptions{}); e != nil {
		t.Fatal(e)
	}
	if _, e = adapter.Read(ctx, spec); !errors.Is(e, ErrWorkloadConflict) {
		t.Fatal("foreign labels adopted")
	}
}

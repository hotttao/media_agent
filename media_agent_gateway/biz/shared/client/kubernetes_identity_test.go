package client

import (
	"context"
	"errors"
	apps "k8s.io/api/apps/v1"
	auth "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kt "k8s.io/client-go/testing"
	d "media_agent/media_agent_gateway/biz/domain/agent_instance"
	"testing"
	"time"
)

func TestKubernetesIdentityUsesBoundTokenAndNativeOwnerChain(t *testing.T) {
	ctx := context.Background()
	labels := map[string]string{"runtime.media-agent.io/workspace-id": "ws", "runtime.media-agent.io/config-id": "cfg", "runtime.media-agent.io/environment-id": "env", "runtime.media-agent.io/agent-type": "codex", "apps.kubernetes.io/pod-index": "0"}
	controller := true
	replicas := int32(2)
	started := metav1.NewTime(time.Now().UTC().Truncate(time.Second))
	pod := &core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "agents-0", Namespace: "test", UID: types.UID("pod-uid"), Labels: labels, OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "agents", UID: types.UID("sts-uid"), Controller: &controller}}}, Spec: core.PodSpec{ServiceAccountName: "agent", Hostname: "agents-0", Subdomain: "agents", Containers: []core.Container{{Name: "agent", Ports: []core.ContainerPort{{Name: "http", ContainerPort: 3001}}}}}, Status: core.PodStatus{ContainerStatuses: []core.ContainerStatus{{Name: "agent", ContainerID: "containerd://one", State: core.ContainerState{Running: &core.ContainerStateRunning{StartedAt: started}}}}}}
	sts := &apps.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "agents", Namespace: "test", UID: types.UID("sts-uid"), OwnerReferences: []metav1.OwnerReference{{APIVersion: "runtime.media-agent.io/v1alpha1", Kind: "AgentWorkload", Name: "workload", UID: types.UID("cr-uid"), Controller: &controller}}}, Spec: apps.StatefulSetSpec{Replicas: &replicas, ServiceName: "agents"}}
	workload := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "runtime.media-agent.io/v1alpha1", "kind": "AgentWorkload", "metadata": map[string]any{"name": "workload", "namespace": "test", "uid": "cr-uid", "labels": map[string]any{"runtime.media-agent.io/workspace-id": "ws", "runtime.media-agent.io/config-id": "cfg", "runtime.media-agent.io/environment-id": "env"}}}}
	api := kubefake.NewClientset(pod, sts)
	audience := "media-agent-runtime-bootstrap"
	tokenPod := "pod-uid"
	api.PrependReactor("create", "tokenreviews", func(action kt.Action) (bool, runtime.Object, error) {
		req := action.(kt.CreateAction).GetObject().(*auth.TokenReview)
		if req.Spec.Token != "proof" || len(req.Spec.Audiences) != 1 || req.Spec.Audiences[0] != "media-agent-runtime-bootstrap" {
			t.Fatal("wrong TokenReview")
		}
		return true, &auth.TokenReview{Status: auth.TokenReviewStatus{Authenticated: true, Audiences: []string{audience}, User: auth.UserInfo{Username: "system:serviceaccount:test:agent", Extra: map[string]auth.ExtraValue{"authentication.kubernetes.io/pod-uid": {tokenPod}, "authentication.kubernetes.io/pod-name": {"agents-0"}}}}}, nil
	})
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), workload)
	identity := &KubernetesIdentity{EnvironmentID: "env", Namespace: "test", API: api, Dynamic: dyn}
	locator := d.ResourceLocator{EnvironmentID: "env", WorkspaceID: "ws", AgentConfigID: "cfg", ReplicaID: "0", Namespace: "test", ResourceName: "agents-0"}
	facts, err := identity.Resolve(ctx, locator, "proof")
	if err != nil || facts.Ready || !facts.Running || !facts.Eligible || facts.Endpoint != "http://agents-0.agents.test.svc.cluster.local:3001" {
		t.Fatalf("bootstrap must accept running before Ready: %+v %v", facts, err)
	}
	tokenPod = "other-pod"
	if _, err = identity.Resolve(ctx, locator, "proof"); !errors.Is(err, ErrNativeIdentity) {
		t.Fatalf("foreign pod token accepted: %v", err)
	}
	tokenPod = "pod-uid"
	audience = "other-audience"
	if _, err = identity.Resolve(ctx, locator, "proof"); !errors.Is(err, ErrNativeIdentity) {
		t.Fatalf("foreign audience accepted: %v", err)
	}
	audience = "media-agent-runtime-bootstrap"
	old := d.Instance{ResourceUID: "pod-uid", ResourceGeneration: facts.Generation, ResourceName: "agents-0", Namespace: "test"}
	if ended, err := identity.Terminated(ctx, old); err != nil || ended {
		t.Fatal("NotReady is not termination")
	}
	pod.Status.ContainerStatuses[0].LastTerminationState.Terminated = &core.ContainerStateTerminated{ContainerID: "containerd://other", FinishedAt: started}
	if _, err = api.CoreV1().Pods("test").UpdateStatus(ctx, pod, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if ended, _ := identity.Terminated(ctx, old); ended {
		t.Fatal("different generation proved termination")
	}
	pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.ContainerID = "containerd://one"
	if _, err = api.CoreV1().Pods("test").UpdateStatus(ctx, pod, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if ended, err := identity.Terminated(ctx, old); err != nil || !ended {
		t.Fatalf("exact native termination not recognized: %v", err)
	}
	if err = api.CoreV1().Pods("test").Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if ended, _ := identity.Terminated(ctx, old); ended {
		t.Fatal("missing Pod proved termination")
	}
	if err = dyn.Resource(schema.GroupVersionResource{Group: "runtime.media-agent.io", Version: "v1alpha1", Resource: "agentworkloads"}).Namespace("test").Delete(ctx, "workload", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
}

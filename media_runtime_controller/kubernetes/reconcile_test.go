package kubernetes

import (
	"context"
	"errors"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	contract "media_agent/hertz_infra/agentworkload"
	"testing"
)

func fixture() (*unstructured.Unstructured, contract.Spec) {
	s := contract.Spec{SessionExchange: "sessions.custom", MetadataExchange: "metadata.custom", Driver: "kubernetes", Namespace: "agents", EnvironmentID: "env1", WorkspaceID: "ws1", AgentConfigID: "cfg1", AgentType: "codex", Image: "agent:test", Replicas: 2, Port: 3001, CPU: "1", Memory: "2Gi", StorageSize: "10Gi", GatewayURL: "http://gateway:8006", SharedSecret: "runtime-shared"}
	n := contract.Name(s.EnvironmentID, s.WorkspaceID)
	s.ClaimName = n + "-workspace"
	s.IdentitySecret = n + "-identity"
	v := &unstructured.Unstructured{Object: map[string]any{"apiVersion": contract.APIVersion, "kind": "AgentWorkload", "spec": s.Object()}}
	v.SetName(n)
	v.SetNamespace(s.Namespace)
	v.SetUID(types.UID("owned-cr"))
	v.SetLabels(s.Labels())
	return v, s
}
func TestResourceOnlyReconcileRetainsSharedWorkspace(t *testing.T) {
	ctx := context.Background()
	cr, s := fixture()
	api := fake.NewClientset()
	r := Reconciler{API: api, Policy: Policy{Namespace: s.Namespace, SharedSecret: s.SharedSecret, ServiceAccount: "media-agent-runtime", Images: map[string]string{"codex": s.Image}}}
	if e := r.Reconcile(ctx, cr); e != nil {
		t.Fatal(e)
	}
	sts, e := api.AppsV1().StatefulSets(s.Namespace).Get(ctx, cr.GetName(), metav1.GetOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if *sts.Spec.Replicas != 2 || sts.Spec.ServiceName != cr.GetName() || sts.Spec.PodManagementPolicy != "Parallel" || len(sts.Spec.VolumeClaimTemplates) != 0 || sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenScaled != "Retain" {
		t.Fatalf("wrong StatefulSet: %+v", sts.Spec)
	}
	if sts.Spec.Template.Spec.AutomountServiceAccountToken == nil || *sts.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Fatal("Agent has automatic Kubernetes credentials")
	}
	c := sts.Spec.Template.Spec.Containers[0]
	if c.StartupProbe == nil || c.ReadinessProbe == nil || c.LivenessProbe == nil {
		t.Fatal("missing native probes")
	}
	foundExchanges := 0
	for _, env := range c.Env {
		if env.Name == "MEDIA_AGENTS_SESSION_EXCHANGE" && env.Value == s.SessionExchange || env.Name == "AGENT_METADATA_EXCHANGE" && env.Value == s.MetadataExchange {
			foundExchanges++
		}
		if env.Name == "WORKSPACE_ID" {
			t.Fatal("legacy CP pair must not be enabled")
		}
	}
	if foundExchanges != 2 {
		t.Fatal("configured Broker exchanges not delivered")
	}
	svc, _ := api.CoreV1().Services(s.Namespace).Get(ctx, cr.GetName(), metav1.GetOptions{})
	if svc.Spec.ClusterIP != "None" || !svc.Spec.PublishNotReadyAddresses {
		t.Fatal("missing stable headless addressing")
	}
	pvc, _ := api.CoreV1().PersistentVolumeClaims(s.Namespace).Get(ctx, s.ClaimName, metav1.GetOptions{})
	if len(pvc.OwnerReferences) != 0 || len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != core.ReadWriteMany {
		t.Fatal("shared claim must survive deletion/scale")
	}
	api.ClearActions()
	if e = r.Reconcile(ctx, cr); e != nil {
		t.Fatal(e)
	}
	for _, a := range api.Actions() {
		if a.GetVerb() != "get" {
			t.Fatalf("idempotent reconcile wrote %s %s", a.GetVerb(), a.GetResource())
		}
	}
	s.Replicas = 0
	cr.Object["spec"] = s.Object()
	if e = r.Reconcile(ctx, cr); e != nil {
		t.Fatal(e)
	}
	sts, _ = api.AppsV1().StatefulSets(s.Namespace).Get(ctx, cr.GetName(), metav1.GetOptions{})
	if *sts.Spec.Replicas != 0 {
		t.Fatal("native desired count not applied")
	}
	if _, e = api.CoreV1().PersistentVolumeClaims(s.Namespace).Get(ctx, s.ClaimName, metav1.GetOptions{}); e != nil {
		t.Fatal("scale-to-zero deleted workspace")
	}
	for _, a := range api.Actions() {
		if a.GetResource().Resource == "pods" {
			t.Fatal("Controller read/probed Agent Pod")
		}
	}
}
func TestRefusesForeignResourcesAndDriver(t *testing.T) {
	ctx := context.Background()
	cr, s := fixture()
	api := fake.NewClientset(&core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: s.ClaimName, Namespace: s.Namespace}})
	r := Reconciler{API: api, Policy: Policy{Namespace: s.Namespace, SharedSecret: s.SharedSecret, Images: map[string]string{"codex": s.Image}}}
	if !errors.Is(r.Reconcile(ctx, cr), contract.ErrConflict) {
		t.Fatal("adopted foreign claim")
	}
	s.Driver = "docker"
	cr.Object["spec"] = s.Object()
	if !errors.Is(r.Reconcile(ctx, cr), contract.ErrInvalid) {
		t.Fatal("accepted Docker in Kubernetes Controller")
	}
}

func TestBoundPVRetainsAndRejectsWrongClaim(t *testing.T) {
	ctx := context.Background()
	cr, s := fixture()
	api := fake.NewClientset()
	r := Reconciler{API: api, Policy: Policy{Namespace: s.Namespace, SharedSecret: s.SharedSecret, ServiceAccount: "media-agent-runtime", Images: map[string]string{"codex": s.Image}}}
	if e := r.Reconcile(ctx, cr); e != nil {
		t.Fatal(e)
	}
	claim, e := api.CoreV1().PersistentVolumeClaims(s.Namespace).Get(ctx, s.ClaimName, metav1.GetOptions{})
	if e != nil {
		t.Fatal(e)
	}
	claim.UID = types.UID("claim-uid")
	claim.Spec.VolumeName = "owned-volume"
	if _, e = api.CoreV1().PersistentVolumeClaims(s.Namespace).Update(ctx, claim, metav1.UpdateOptions{}); e != nil {
		t.Fatal(e)
	}
	pv := &core.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "owned-volume"}, Spec: core.PersistentVolumeSpec{ClaimRef: &core.ObjectReference{Namespace: s.Namespace, Name: s.ClaimName, UID: claim.UID}, PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimDelete}}
	if _, e = api.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{}); e != nil {
		t.Fatal(e)
	}
	if e = r.Reconcile(ctx, cr); e != nil {
		t.Fatal(e)
	}
	pv, e = api.CoreV1().PersistentVolumes().Get(ctx, pv.Name, metav1.GetOptions{})
	if e != nil || pv.Spec.PersistentVolumeReclaimPolicy != core.PersistentVolumeReclaimRetain {
		t.Fatal("PV retention not enforced")
	}
	pv.Spec.ClaimRef.UID = "another-claim"
	if _, e = api.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{}); e != nil {
		t.Fatal(e)
	}
	if e = r.Reconcile(ctx, cr); !errors.Is(e, contract.ErrConflict) {
		t.Fatal("modified foreign bound PV")
	}
}

package kubernetes

import (
	"context"
	"encoding/json"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	contract "media_agent/hertz_infra/agentworkload"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"testing"
	"time"
)

func TestLiveKubernetesResources(t *testing.T) {
	if os.Getenv("MEDIA_RUNTIME_KUBERNETES_TEST") != "1" {
		t.Skip("dedicated live Kubernetes resource test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	file := os.Getenv("KUBECONFIG")
	if file == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			t.Fatal(e)
		}
		file = filepath.Join(home, ".kube", "config")
	}
	cfg, e := clientcmd.BuildConfigFromFlags("", file)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Timeout = 10 * time.Second
	api, e := kubernetes.NewForConfig(cfg)
	if e != nil {
		t.Fatal(e)
	}
	dyn, e := dynamic.NewForConfig(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defs := dyn.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})
	definition, e := defs.Get(ctx, "agentworkloads."+contract.Group, metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		body, e := os.ReadFile("../deploy/crd.yaml")
		if e != nil {
			t.Fatal(e)
		}
		body, e = yaml.YAMLToJSON(body)
		if e != nil {
			t.Fatal(e)
		}
		definition = &unstructured.Unstructured{}
		if json.Unmarshal(body, &definition.Object) != nil {
			t.Fatal("invalid CRD YAML")
		}
		definition.SetLabels(map[string]string{"media-agent-test": "p5"})
		definition, e = defs.Create(ctx, definition, metav1.CreateOptions{})
		if e != nil {
			t.Fatal(e)
		}
		uid := definition.GetUID()
		defer func() {
			cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
			defer done()
			current, e := defs.Get(cleanup, definition.GetName(), metav1.GetOptions{})
			if e == nil && current.GetUID() == uid {
				_ = defs.Delete(cleanup, current.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
			}
		}()
	} else if e != nil {
		t.Fatal(e)
	}
	ns, e := api.CoreV1().Namespaces().Create(ctx, &core.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "media-p5-", Labels: map[string]string{"media-agent-test": "p5"}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		_ = api.CoreV1().Namespaces().Delete(cleanup, ns.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}})
	}()
	cr, s := fixture()
	s.Namespace = ns.Name
	s.Image = "traefik/whoami:v1.11.0"
	if image := os.Getenv("MEDIA_RUNTIME_KUBERNETES_TEST_IMAGE"); image != "" {
		s.Image = image
	}
	s.Port = 80
	s.CPU = "100m"
	s.Memory = "64Mi"
	s.StorageSize = "20Mi"
	s.StorageClass = os.Getenv("MEDIA_RUNTIME_KUBERNETES_STORAGE_CLASS")
	if s.StorageClass == "" {
		s.StorageClass = "nfs-nap"
	}
	cr.SetNamespace(ns.Name)
	cr.SetUID("")
	cr.Object["spec"] = s.Object()
	resource := dyn.Resource(schema.GroupVersionResource{Group: contract.Group, Version: "v1alpha1", Resource: "agentworkloads"}).Namespace(ns.Name)
	wait := func(label string, predicate func() bool) {
		t.Helper()
		for ctx.Err() == nil {
			if predicate() {
				return
			}
			time.Sleep(time.Second)
		}
		t.Fatalf("timeout waiting for %s", label)
	}
	wait("CRD established", func() bool {
		v, e := defs.Get(ctx, definition.GetName(), metav1.GetOptions{})
		if e != nil {
			return false
		}
		conditions, _, _ := unstructured.NestedSlice(v.Object, "status", "conditions")
		for _, v := range conditions {
			row, _ := v.(map[string]any)
			if row["type"] == "Established" && row["status"] == "True" {
				return true
			}
		}
		return false
	})
	cr, e = resource.Create(ctx, cr, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = api.CoreV1().ServiceAccounts(ns.Name).Create(ctx, &core.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "media-agent-runtime"}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	for name, data := range map[string]map[string][]byte{s.IdentitySecret: {"agent-api-token": []byte("native-resource-fixture-token")}, s.SharedSecret: {"database-url": []byte("fixture-db"), "rabbitmq-url": []byte("fixture-broker"), "runtime-public-keys.json": []byte("{}")}} {
		_, e = api.CoreV1().Secrets(ns.Name).Create(ctx, &core.Secret{ObjectMeta: metav1.ObjectMeta{Name: name}, Data: data}, metav1.CreateOptions{})
		if e != nil {
			t.Fatal(e)
		}
	}
	r := Reconciler{API: api, Policy: Policy{Namespace: ns.Name, SharedSecret: s.SharedSecret, ServiceAccount: "media-agent-runtime", Images: map[string]string{"codex": s.Image}}}
	if e = r.Reconcile(ctx, cr); e != nil {
		t.Fatal(e)
	}
	var claim *core.PersistentVolumeClaim

	claim, e = api.CoreV1().PersistentVolumeClaims(ns.Name).Get(ctx, s.ClaimName, metav1.GetOptions{})
	if e != nil {
		t.Fatal(e)
	}
	claimUID := claim.UID
	fullyReady := os.Getenv("MEDIA_RUNTIME_KUBERNETES_REQUIRE_READY") == "1"
	var sts *apps.StatefulSet
	if fullyReady {
		wait("RWX volume bound", func() bool {
			claim, e = api.CoreV1().PersistentVolumeClaims(ns.Name).Get(ctx, s.ClaimName, metav1.GetOptions{})
			return e == nil && claim.Status.Phase == core.ClaimBound
		})
		volumeName := claim.Spec.VolumeName
		defer func() {
			cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
			defer done()
			pv, e := api.CoreV1().PersistentVolumes().Get(cleanup, volumeName, metav1.GetOptions{})
			if e == nil && pv.Spec.ClaimRef != nil && pv.Spec.ClaimRef.UID == claimUID {
				_, _ = api.CoreV1().PersistentVolumes().Patch(cleanup, volumeName, types.MergePatchType, []byte(`{"spec":{"persistentVolumeReclaimPolicy":"Delete"}}`), metav1.PatchOptions{})
			}
		}()
		if e = r.Reconcile(ctx, cr); e != nil {
			t.Fatal(e)
		}
		pv, e := api.CoreV1().PersistentVolumes().Get(ctx, volumeName, metav1.GetOptions{})
		if e != nil || pv.Spec.PersistentVolumeReclaimPolicy != core.PersistentVolumeReclaimRetain {
			t.Fatalf("shared PV not Retain: %v", e)
		}
		wait("two Ready native replicas", func() bool {
			sts, e = api.AppsV1().StatefulSets(ns.Name).Get(ctx, cr.GetName(), metav1.GetOptions{})
			return e == nil && sts.Status.ReadyReplicas == 2
		})
		pods, e := api.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
		if e != nil {
			t.Fatal(e)
		}
		for _, pod := range pods.Items {
			if pod.Spec.Hostname != pod.Name || pod.Spec.Subdomain != cr.GetName() {
				t.Fatal("unstable network identity")
			}
		}
	} else {
		sts, e = api.AppsV1().StatefulSets(ns.Name).Get(ctx, cr.GetName(), metav1.GetOptions{})
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("native resources created; PVC phase=%s; full Ready/RWX/PV test requires MEDIA_RUNTIME_KUBERNETES_REQUIRE_READY=1 and running RWX provisioner", claim.Status.Phase)
	}
	apiBefore := sts.ResourceVersion
	if e = r.Reconcile(ctx, cr); e != nil {
		t.Fatal(e)
	}
	sts, e = api.AppsV1().StatefulSets(ns.Name).Get(ctx, cr.GetName(), metav1.GetOptions{})
	if e != nil || sts.ResourceVersion != apiBefore {
		t.Fatalf("API defaulting causes repeated template updates: %v", e)
	}
	s.Replicas = 0
	cr.Object["spec"] = s.Object()
	cr, e = resource.Update(ctx, cr, metav1.UpdateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Reconcile(ctx, cr); e != nil {
		t.Fatal(e)
	}
	wait("native scale-to-zero", func() bool {
		pods, e := api.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
		return e == nil && len(pods.Items) == 0
	})
	claim, e = api.CoreV1().PersistentVolumeClaims(ns.Name).Get(ctx, s.ClaimName, metav1.GetOptions{})
	if e != nil || claim.UID != claimUID {
		t.Fatalf("scale lost shared workspace: %v", e)
	}
	s.Replicas = 2
	cr.Object["spec"] = s.Object()
	cr, e = resource.Update(ctx, cr, metav1.UpdateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Reconcile(ctx, cr); e != nil {
		t.Fatal(e)
	}
	if fullyReady {
		wait("native restoration", func() bool {
			sts, e = api.AppsV1().StatefulSets(ns.Name).Get(ctx, cr.GetName(), metav1.GetOptions{})
			return e == nil && sts.Status.ReadyReplicas == 2
		})
	} else {
		sts, e = api.AppsV1().StatefulSets(ns.Name).Get(ctx, cr.GetName(), metav1.GetOptions{})
		if e != nil || *sts.Spec.Replicas != 2 {
			t.Fatalf("native restoration intent missing: %v", e)
		}
	}
	claim, e = api.CoreV1().PersistentVolumeClaims(ns.Name).Get(ctx, s.ClaimName, metav1.GetOptions{})
	if e != nil || claim.UID != claimUID {
		t.Fatal("restart changed persistent workspace identity")
	}
	t.Logf("live namespace=%s StatefulSet/headless/RWX claim identity/native replicas/zero/restore verified (full Ready=%t)", ns.Name, fullyReady)
}

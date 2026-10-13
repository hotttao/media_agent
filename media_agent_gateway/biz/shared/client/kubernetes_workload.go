package client

import (
	"context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	contract "media_agent/hertz_infra/agentworkload"
	"strconv"
)

var workloadGVR = schema.GroupVersionResource{Group: contract.Group, Version: "v1alpha1", Resource: "agentworkloads"}

type KubernetesWorkloads struct {
	Native                   kubernetes.Interface
	API                      dynamic.Interface
	EnvironmentID, Namespace string
}

func (k KubernetesWorkloads) Read(ctx context.Context, s contract.Spec) (*NativeWorkload, error) {
	if s.Driver != "kubernetes" || s.EnvironmentID != k.EnvironmentID || s.Namespace != k.Namespace {
		return nil, ErrWorkloadConflict
	}
	v, e := k.API.Resource(workloadGVR).Namespace(k.Namespace).Get(ctx, contract.Name(s.EnvironmentID, s.WorkspaceID), metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		return nil, ErrWorkloadAbsent
	}
	if e != nil {
		return nil, ErrNativeUnavailable
	}
	actual, e := contract.Decode(v.Object["spec"])
	if e != nil {
		return nil, e
	}
	for key, value := range s.Labels() {
		if v.GetLabels()[key] != value {
			return nil, ErrWorkloadConflict
		}
	}
	if actual.WorkspaceID != s.WorkspaceID || actual.AgentConfigID != s.AgentConfigID || actual.EnvironmentID != s.EnvironmentID {
		return nil, ErrWorkloadConflict
	}
	a := v.GetAnnotations()
	version, e := strconv.ParseInt(a[contract.TargetVersionAnnotation], 10, 64)
	if e != nil || version < 1 {
		return nil, ErrWorkloadConflict
	}
	out := &NativeWorkload{Spec: actual, Revision: v.GetResourceVersion(), Version: version, Operation: a[contract.DrainAnnotation]}
	if out.Operation != "" {
		n, e := strconv.ParseInt(a[contract.PendingReplicasAnnotation], 10, 32)
		if e != nil || n < 0 || n > 64 {
			return nil, ErrWorkloadConflict
		}
		out.PendingReplicas = int32(n)
		out.PendingVersion, e = strconv.ParseInt(a[contract.PendingVersionAnnotation], 10, 64)
		if e != nil || out.PendingVersion < 1 {
			return nil, ErrWorkloadConflict
		}
	}

	if k.Native != nil {
		sts, e := k.Native.AppsV1().StatefulSets(k.Namespace).Get(ctx, v.GetName(), metav1.GetOptions{})
		if e != nil && !apierrors.IsNotFound(e) {
			return nil, ErrNativeUnavailable
		}
		if e == nil {
			owner := metav1.GetControllerOf(sts)
			if owner == nil || owner.UID != v.GetUID() {
				return nil, ErrWorkloadConflict
			}
			out.ReadyReplicas = sts.Status.ReadyReplicas
			out.Settled = sts.Spec.Replicas != nil && *sts.Spec.Replicas == actual.Replicas && sts.Status.ObservedGeneration >= sts.Generation
			pods, e := k.Native.CoreV1().Pods(k.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(actual.Labels()).String()})
			if e != nil {
				return nil, ErrNativeUnavailable
			}
			for _, pod := range pods.Items {
				ordinal, e := strconv.Atoi(pod.Labels["apps.kubernetes.io/pod-index"])
				if e != nil || ordinal >= int(actual.Replicas) {
					out.Settled = false
				}
			}

		}
	}
	return out, nil
}
func (k KubernetesWorkloads) Save(ctx context.Context, s *NativeWorkload) error {
	if s.Spec.Validate() != nil || s.Spec.Driver != "kubernetes" || s.Spec.Namespace != k.Namespace || s.Spec.EnvironmentID != k.EnvironmentID || s.Version < 1 {
		return ErrWorkloadConflict
	}
	annotations := map[string]string{contract.TargetVersionAnnotation: strconv.FormatInt(s.Version, 10)}
	if s.Operation != "" {
		annotations[contract.DrainAnnotation] = s.Operation
		annotations[contract.PendingReplicasAnnotation] = strconv.Itoa(int(s.PendingReplicas))
		annotations[contract.PendingVersionAnnotation] = strconv.FormatInt(s.PendingVersion, 10)
	}
	obj := &unstructured.Unstructured{Object: map[string]any{"apiVersion": contract.APIVersion, "kind": "AgentWorkload", "spec": s.Spec.Object()}}
	obj.SetName(contract.Name(s.Spec.EnvironmentID, s.Spec.WorkspaceID))
	obj.SetNamespace(k.Namespace)
	obj.SetLabels(s.Spec.Labels())
	obj.SetAnnotations(annotations)
	api := k.API.Resource(workloadGVR).Namespace(k.Namespace)
	if s.Revision == "" {
		_, e := api.Create(ctx, obj, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(e) {
			return ErrWorkloadConflict
		}
		return e
	}
	current, e := api.Get(ctx, obj.GetName(), metav1.GetOptions{})
	if e != nil {
		return e
	}
	if current.GetResourceVersion() != s.Revision || current.GetDeletionTimestamp() != nil {
		return ErrWorkloadConflict
	}
	for key, value := range s.Spec.Labels() {
		if current.GetLabels()[key] != value {
			return ErrWorkloadConflict
		}
	}
	// Retain server metadata, finalizers and status; CAS only fields owned by Gateway.
	current.Object["spec"] = obj.Object["spec"]
	a := current.GetAnnotations()
	if a == nil {
		a = map[string]string{}
	}
	for _, key := range []string{contract.TargetVersionAnnotation, contract.DrainAnnotation, contract.PendingReplicasAnnotation, contract.PendingVersionAnnotation} {
		delete(a, key)
	}
	for key, value := range annotations {
		a[key] = value
	}
	current.SetAnnotations(a)
	_, e = api.Update(ctx, current, metav1.UpdateOptions{})
	if apierrors.IsConflict(e) {
		return ErrWorkloadConflict
	}
	return e
}

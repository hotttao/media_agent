// Package kubernetes reconciles declared resources only. It never reads Pods,
// probes an Agent, issues runtime commands, or accesses business databases.
package kubernetes

import (
	"context"
	"fmt"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"log"
	contract "media_agent/hertz_infra/agentworkload"
	"reflect"
	"time"
)

type Policy struct {
	Namespace, SharedSecret, ServiceAccount string
	Images                                  map[string]string
}
type Reconciler struct {
	API    kubernetes.Interface
	Policy Policy
}

func owned(object metav1.Object, cr *unstructured.Unstructured, s contract.Spec) bool {
	for key, value := range s.Labels() {
		if object.GetLabels()[key] != value {
			return false
		}
	}
	owner := metav1.GetControllerOf(object)
	return owner != nil && owner.UID == cr.GetUID() && owner.Kind == "AgentWorkload" && owner.APIVersion == contract.APIVersion
}
func (r *Reconciler) Reconcile(ctx context.Context, cr *unstructured.Unstructured) error {
	if cr.GetDeletionTimestamp() != nil {
		return nil
	}
	s, e := contract.Decode(cr.Object["spec"])
	if e != nil {
		return e
	}
	n := contract.Name(s.EnvironmentID, s.WorkspaceID)
	if s.Driver != "kubernetes" || s.Namespace != cr.GetNamespace() || cr.GetNamespace() != r.Policy.Namespace || cr.GetUID() == "" || cr.GetName() != n || s.ClaimName != n+"-workspace" || s.IdentitySecret != n+"-identity" || s.SharedSecret != r.Policy.SharedSecret || r.Policy.Images[s.AgentType] != s.Image {
		return contract.ErrInvalid
	}
	for k, v := range s.Labels() {
		if cr.GetLabels()[k] != v {
			return contract.ErrConflict
		}
	}
	quantities := []string{s.CPU, s.Memory, s.StorageSize}
	parsed := make([]resource.Quantity, 3)
	for i, v := range quantities {
		parsed[i], e = resource.ParseQuantity(v)
		if e != nil || parsed[i].Sign() <= 0 {
			return contract.ErrInvalid
		}
	}
	owner := *metav1.NewControllerRef(cr, cr.GroupVersionKind())
	labels := s.Labels()
	meta := metav1.ObjectMeta{Name: n, Namespace: cr.GetNamespace(), Labels: labels, OwnerReferences: []metav1.OwnerReference{owner}}
	// The shared claim deliberately has no controller ownerReference or deletion finalizer.
	claim := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: s.ClaimName, Namespace: cr.GetNamespace(), Labels: labels}, Spec: core.PersistentVolumeClaimSpec{AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteMany}, Resources: core.VolumeResourceRequirements{Requests: core.ResourceList{core.ResourceStorage: parsed[2]}}}}
	if s.StorageClass != "" {
		claim.Spec.StorageClassName = &s.StorageClass
	}
	existing, e := r.API.CoreV1().PersistentVolumeClaims(cr.GetNamespace()).Get(ctx, s.ClaimName, metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		existing, e = r.API.CoreV1().PersistentVolumeClaims(cr.GetNamespace()).Create(ctx, claim, metav1.CreateOptions{})
	} else if e == nil {
		for k, v := range labels {
			if existing.Labels[k] != v {
				return contract.ErrConflict
			}
		}
		if len(existing.OwnerReferences) != 0 || !reflect.DeepEqual(existing.Spec.AccessModes, claim.Spec.AccessModes) || existing.Spec.Resources.Requests.Storage().Cmp(parsed[2]) != 0 {
			return contract.ErrConflict
		}
		if s.StorageClass != "" && (existing.Spec.StorageClassName == nil || *existing.Spec.StorageClassName != s.StorageClass) {
			return contract.ErrConflict
		}
	}
	if e != nil {
		return e
	}
	if existing.Spec.VolumeName != "" {
		pv, e := r.API.CoreV1().PersistentVolumes().Get(ctx, existing.Spec.VolumeName, metav1.GetOptions{})
		if e != nil {
			return e
		}
		if pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.UID != existing.UID || pv.Spec.ClaimRef.Namespace != cr.GetNamespace() || pv.Spec.ClaimRef.Name != s.ClaimName {
			return contract.ErrConflict
		}
		if pv.Spec.PersistentVolumeReclaimPolicy != core.PersistentVolumeReclaimRetain {
			_, e = r.API.CoreV1().PersistentVolumes().Patch(ctx, pv.Name, types.MergePatchType, []byte(`{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}`), metav1.PatchOptions{})
			if e != nil {
				return e
			}
		}
	}
	service := &core.Service{ObjectMeta: meta, Spec: core.ServiceSpec{ClusterIP: core.ClusterIPNone, PublishNotReadyAddresses: true, Selector: labels, Ports: []core.ServicePort{{Name: "http", Protocol: core.ProtocolTCP, Port: s.Port, TargetPort: intstr.FromString("http")}}}}
	current, e := r.API.CoreV1().Services(cr.GetNamespace()).Get(ctx, n, metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		_, e = r.API.CoreV1().Services(cr.GetNamespace()).Create(ctx, service, metav1.CreateOptions{})
	} else if e == nil {
		if !owned(current, cr, s) || current.Spec.ClusterIP != core.ClusterIPNone {
			return contract.ErrConflict
		}
		if !reflect.DeepEqual(current.Spec.Selector, service.Spec.Selector) || !reflect.DeepEqual(current.Spec.Ports, service.Spec.Ports) || !current.Spec.PublishNotReadyAddresses {
			current.Spec.Selector = service.Spec.Selector
			current.Spec.Ports = service.Spec.Ports
			current.Spec.PublishNotReadyAddresses = true
			_, e = r.API.CoreV1().Services(cr.GetNamespace()).Update(ctx, current, metav1.UpdateOptions{})
		}
	}
	if e != nil {
		return e
	}
	pod := podSpec(s, r.Policy.ServiceAccount, parsed[0], parsed[1])
	desired := &apps.StatefulSet{ObjectMeta: meta, Spec: apps.StatefulSetSpec{Replicas: &s.Replicas, ServiceName: n, PodManagementPolicy: apps.ParallelPodManagement, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: core.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: pod}, UpdateStrategy: apps.StatefulSetUpdateStrategy{Type: apps.OnDeleteStatefulSetStrategyType}, PersistentVolumeClaimRetentionPolicy: &apps.StatefulSetPersistentVolumeClaimRetentionPolicy{WhenDeleted: apps.RetainPersistentVolumeClaimRetentionPolicyType, WhenScaled: apps.RetainPersistentVolumeClaimRetentionPolicyType}}}
	sts, e := r.API.AppsV1().StatefulSets(cr.GetNamespace()).Get(ctx, n, metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		_, e = r.API.AppsV1().StatefulSets(cr.GetNamespace()).Create(ctx, desired, metav1.CreateOptions{})
	} else if e == nil {
		if !owned(sts, cr, s) || sts.Spec.ServiceName != n || sts.Spec.PodManagementPolicy != apps.ParallelPodManagement || !reflect.DeepEqual(sts.Spec.Selector, desired.Spec.Selector) {
			return contract.ErrConflict
		}
		// Workload template changes are explicit infrastructure changes. OnDelete keeps
		// existing executing Pods intact; the Gateway coordinates their retirement.
		if sts.Spec.Replicas == nil || *sts.Spec.Replicas != s.Replicas || !samePod(sts.Spec.Template.Spec, pod) {
			sts.Spec.Replicas = &s.Replicas
			sts.Spec.Template = desired.Spec.Template
			sts.Spec.UpdateStrategy = desired.Spec.UpdateStrategy
			sts.Spec.PersistentVolumeClaimRetentionPolicy = desired.Spec.PersistentVolumeClaimRetentionPolicy
			_, e = r.API.AppsV1().StatefulSets(cr.GetNamespace()).Update(ctx, sts, metav1.UpdateOptions{})
		}
	}
	return e
}
func samePod(a, b core.PodSpec) bool {
	// API defaulting fills omitted fields. Compare only the fields this controller owns.
	return reflect.DeepEqual(a.Containers, b.Containers) && reflect.DeepEqual(a.Volumes, b.Volumes) && a.ServiceAccountName == b.ServiceAccountName && a.AutomountServiceAccountToken != nil && !*a.AutomountServiceAccountToken
}
func podSpec(s contract.Spec, account string, cpu, memory resource.Quantity) core.PodSpec {
	no := false
	grace := int64(120)
	mode := int32(0400)
	expiration := int64(3600)
	env := []core.EnvVar{{Name: "PORT", Value: fmt.Sprint(s.Port)}, {Name: "WORKSPACE_DIR", Value: "/workspace"}, {Name: "RUNTIME_WORKSPACE_ID", Value: s.WorkspaceID}, {Name: "AGENT_INSTANCE_ENABLED", Value: "true"}, {Name: "AGENT_METADATA_ENABLED", Value: "true"}, {Name: "AGENT_CONFIG_ID", Value: s.AgentConfigID}, {Name: "AGENT_GATEWAY_URL", Value: s.GatewayURL}, {Name: "AGENT_ENVIRONMENT_ID", Value: s.EnvironmentID}, {Name: "AGENT_BOOTSTRAP_PROOF_FILE", Value: "/var/run/secrets/media-agent/identity-token"}, {Name: "RUNTIME_PUBLIC_KEYS_FILE", Value: "/run/secrets/media-agent/runtime-public-keys.json"}}
	if s.SessionExchange != "" {
		env = append(env, core.EnvVar{Name: "MEDIA_AGENTS_SESSION_EXCHANGE", Value: s.SessionExchange})
	}
	if s.MetadataExchange != "" {
		env = append(env, core.EnvVar{Name: "AGENT_METADATA_EXCHANGE", Value: s.MetadataExchange})
	}
	for envName, field := range map[string]string{"POD_NAME": "metadata.name", "POD_NAMESPACE": "metadata.namespace", "POD_UID": "metadata.uid", "AGENT_REPLICA_ID": "metadata.labels['apps.kubernetes.io/pod-index']"} {
		env = append(env, core.EnvVar{Name: envName, ValueFrom: &core.EnvVarSource{FieldRef: &core.ObjectFieldSelector{APIVersion: "v1", FieldPath: field}}})
	}
	// Sort the generated variables so reconcile is stable across process instances.
	sortEnv(env)
	env = append(env, core.EnvVar{Name: "AGENT_API_TOKEN", ValueFrom: &core.EnvVarSource{SecretKeyRef: &core.SecretKeySelector{LocalObjectReference: core.LocalObjectReference{Name: s.IdentitySecret}, Key: "agent-api-token"}}}, core.EnvVar{Name: "MEDIA_AGENTS_DB_URL", ValueFrom: &core.EnvVarSource{SecretKeyRef: &core.SecretKeySelector{LocalObjectReference: core.LocalObjectReference{Name: s.SharedSecret}, Key: "database-url"}}}, core.EnvVar{Name: "MEDIA_AGENTS_RABBITMQ_URL", ValueFrom: &core.EnvVarSource{SecretKeyRef: &core.SecretKeySelector{LocalObjectReference: core.LocalObjectReference{Name: s.SharedSecret}, Key: "rabbitmq-url"}}})
	probe := func(period, failure int32) *core.Probe {
		return &core.Probe{ProbeHandler: core.ProbeHandler{HTTPGet: &core.HTTPGetAction{Path: "/health", Port: intstr.FromString("http"), Scheme: core.URISchemeHTTP}}, PeriodSeconds: period, TimeoutSeconds: 3, FailureThreshold: failure, SuccessThreshold: 1}
	}
	return core.PodSpec{ServiceAccountName: account, AutomountServiceAccountToken: &no, TerminationGracePeriodSeconds: &grace, Containers: []core.Container{{Name: "agent", TerminationMessagePath: "/dev/termination-log", TerminationMessagePolicy: core.TerminationMessageReadFile, Image: s.Image, ImagePullPolicy: core.PullIfNotPresent, Ports: []core.ContainerPort{{Name: "http", Protocol: core.ProtocolTCP, ContainerPort: s.Port}}, Env: env, Resources: core.ResourceRequirements{Requests: core.ResourceList{core.ResourceCPU: cpu, core.ResourceMemory: memory}, Limits: core.ResourceList{core.ResourceCPU: cpu, core.ResourceMemory: memory}}, VolumeMounts: []core.VolumeMount{{Name: "workspace", MountPath: "/workspace"}, {Name: "bootstrap", MountPath: "/var/run/secrets/media-agent", ReadOnly: true}, {Name: "runtime-public", MountPath: "/run/secrets/media-agent", ReadOnly: true}}, StartupProbe: probe(5, 60), ReadinessProbe: probe(5, 3), LivenessProbe: probe(10, 3), SecurityContext: &core.SecurityContext{AllowPrivilegeEscalation: &no, Capabilities: &core.Capabilities{Drop: []core.Capability{"ALL"}}}}}, Volumes: []core.Volume{{Name: "workspace", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: s.ClaimName}}}, {Name: "bootstrap", VolumeSource: core.VolumeSource{Projected: &core.ProjectedVolumeSource{DefaultMode: &mode, Sources: []core.VolumeProjection{{ServiceAccountToken: &core.ServiceAccountTokenProjection{Audience: "media-agent-runtime-bootstrap", ExpirationSeconds: &expiration, Path: "identity-token"}}}}}}, {Name: "runtime-public", VolumeSource: core.VolumeSource{Secret: &core.SecretVolumeSource{SecretName: s.SharedSecret, DefaultMode: &mode, Items: []core.KeyToPath{{Key: "runtime-public-keys.json", Path: "runtime-public-keys.json"}}}}}}}
}
func sortEnv(values []core.EnvVar) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j].Name < values[i].Name {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

// Run observes native desired resources only; missed events and restarts converge
// through the same idempotent reconciliation path.
func (r *Reconciler) Run(ctx context.Context, dyn dynamic.Interface) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		list, e := dyn.Resource(schema.GroupVersionResource{Group: contract.Group, Version: "v1alpha1", Resource: "agentworkloads"}).Namespace(r.Policy.Namespace).List(ctx, metav1.ListOptions{})
		if e != nil {
			log.Print("AgentWorkload list unavailable")
		} else {
			for i := range list.Items {
				if e = r.Reconcile(ctx, &list.Items[i]); e != nil {
					log.Printf("resource reconciliation failed workload=%s", list.Items[i].GetName())
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

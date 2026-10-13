package client

import (
	"context"
	"errors"
	"fmt"
	authentication "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	d "media_agent/media_agent_gateway/biz/domain/agent_instance"
	"slices"
	"strconv"
	"strings"
	"time"
)

var ErrNativeIdentity = errors.New("native runtime identity rejected")
var ErrNativeUnavailable = errors.New("native runtime facts unavailable")

type KubernetesIdentity struct {
	EnvironmentID, Namespace string
	API                      kubernetes.Interface
	Dynamic                  dynamic.Interface
}

func NewKubernetesIdentity(environment, namespace, path string) (*KubernetesIdentity, error) {
	var cfg *rest.Config
	var err error
	if path == "" {
		cfg, err = rest.InClusterConfig()
	} else {
		cfg, err = clientcmd.BuildConfigFromFlags("", path)
	}
	if err != nil {
		return nil, ErrNativeUnavailable
	}
	cfg.Timeout = 5 * time.Second
	api, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, ErrNativeUnavailable
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, ErrNativeUnavailable
	}
	return &KubernetesIdentity{environment, namespace, api, dyn}, nil
}
func (k *KubernetesIdentity) Resolve(ctx context.Context, locator d.ResourceLocator, proof string) (d.ResourceFacts, error) {
	if proof == "" || len(proof) > 16<<10 {
		return d.ResourceFacts{}, ErrNativeIdentity
	}
	review, err := k.API.AuthenticationV1().TokenReviews().Create(ctx, &authentication.TokenReview{Spec: authentication.TokenReviewSpec{Token: proof, Audiences: []string{"media-agent-runtime-bootstrap"}}}, metav1.CreateOptions{})
	if err != nil {
		return d.ResourceFacts{}, ErrNativeUnavailable
	}
	if !review.Status.Authenticated || review.Status.Error != "" || !slices.Contains(review.Status.Audiences, "media-agent-runtime-bootstrap") {
		return d.ResourceFacts{}, ErrNativeIdentity
	}
	facts, pod, err := k.facts(ctx, locator)
	if err != nil {
		return facts, err
	}
	uids := review.Status.User.Extra["authentication.kubernetes.io/pod-uid"]
	names := review.Status.User.Extra["authentication.kubernetes.io/pod-name"]
	expected := "system:serviceaccount:" + pod.Namespace + ":" + pod.Spec.ServiceAccountName
	if review.Status.User.Username != expected || len(uids) != 1 || uids[0] != string(pod.UID) || len(names) != 1 || names[0] != pod.Name || !facts.Running || !facts.Eligible {
		return facts, ErrNativeIdentity
	}
	return facts, nil
}
func (k *KubernetesIdentity) Observe(ctx context.Context, v d.Instance) (d.ResourceFacts, error) {
	facts, _, err := k.facts(ctx, d.ResourceLocator{EnvironmentID: v.EnvironmentID, WorkspaceID: v.WorkspaceID, AgentConfigID: v.AgentConfigID, ReplicaID: v.ReplicaID, Namespace: v.Namespace, ResourceName: v.ResourceName, ResourceUID: v.ResourceUID})
	return facts, err
}
func (k *KubernetesIdentity) facts(ctx context.Context, l d.ResourceLocator) (d.ResourceFacts, *core.Pod, error) {
	empty := d.ResourceFacts{}
	if l.EnvironmentID != k.EnvironmentID || l.Namespace != k.Namespace || l.ResourceName == "" {
		return empty, nil, ErrNativeIdentity
	}
	pod, err := k.API.CoreV1().Pods(l.Namespace).Get(ctx, l.ResourceName, metav1.GetOptions{})
	if err != nil {
		return empty, nil, ErrNativeUnavailable
	}
	labels := pod.Labels
	if l.ResourceUID != "" && string(pod.UID) != l.ResourceUID || labels["runtime.media-agent.io/workspace-id"] != l.WorkspaceID || labels["runtime.media-agent.io/config-id"] != l.AgentConfigID || labels["runtime.media-agent.io/environment-id"] != l.EnvironmentID || labels["apps.kubernetes.io/pod-index"] != l.ReplicaID {
		return empty, pod, ErrNativeIdentity
	}
	owner := metav1.GetControllerOf(pod)
	if owner == nil || owner.Kind != "StatefulSet" {
		return empty, pod, ErrNativeIdentity
	}
	sts, err := k.API.AppsV1().StatefulSets(l.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
	if err != nil {
		return empty, pod, ErrNativeUnavailable
	}
	crOwner := metav1.GetControllerOf(sts)
	if sts.UID != owner.UID || crOwner == nil || crOwner.Kind != "AgentWorkload" || crOwner.APIVersion != "runtime.media-agent.io/v1alpha1" {
		return empty, pod, ErrNativeIdentity
	}
	workload, err := k.Dynamic.Resource(schema.GroupVersionResource{Group: "runtime.media-agent.io", Version: "v1alpha1", Resource: "agentworkloads"}).Namespace(l.Namespace).Get(ctx, crOwner.Name, metav1.GetOptions{})
	if err != nil {
		return empty, pod, ErrNativeUnavailable
	}
	if workload.GetUID() != crOwner.UID || workload.GetLabels()["runtime.media-agent.io/workspace-id"] != l.WorkspaceID || workload.GetLabels()["runtime.media-agent.io/config-id"] != l.AgentConfigID || workload.GetLabels()["runtime.media-agent.io/environment-id"] != l.EnvironmentID {
		return empty, pod, ErrNativeIdentity
	}
	ordinal, err := strconv.Atoi(l.ReplicaID)
	if err != nil || ordinal < 0 || pod.Name != fmt.Sprintf("%s-%d", sts.Name, ordinal) || pod.Spec.Hostname != pod.Name || pod.Spec.Subdomain != sts.Spec.ServiceName || pod.Spec.Subdomain == "" {
		return empty, pod, ErrNativeIdentity
	}
	port := int32(0)
	for _, container := range pod.Spec.Containers {
		if container.Name == "agent" {
			for _, value := range container.Ports {
				if value.Name == "http" {
					port = value.ContainerPort
				}
			}
		}
	}
	if port <= 0 || port > 65535 {
		return empty, pod, ErrNativeIdentity
	}
	running := false
	generation := ""
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "agent" && status.State.Running != nil {
			running = true
			generation = status.ContainerID + "@" + status.State.Running.StartedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	ready := false
	for _, condition := range pod.Status.Conditions {
		if condition.Type == core.PodReady && condition.Status == core.ConditionTrue {
			ready = true
		}
	}
	declared, found, e := unstructured.NestedInt64(workload.Object, "spec", "replicas")
	if e != nil {
		return empty, pod, ErrNativeIdentity
	}
	// Legacy P3 resources may lack a replicas projection until P5 provisioning is enabled.
	if !found && sts.Spec.Replicas != nil {
		declared = int64(*sts.Spec.Replicas)
	}
	eligible := sts.Spec.Replicas != nil && ordinal < int(*sts.Spec.Replicas) && ordinal < int(declared) && pod.DeletionTimestamp == nil
	agentType := labels["runtime.media-agent.io/agent-type"]
	if agentType != "codex" && agentType != "claude-code" && agentType != "goose" {
		return empty, pod, ErrNativeIdentity
	}
	l.ResourceUID = string(pod.UID)
	return d.ResourceFacts{Locator: l, Generation: generation, Endpoint: fmt.Sprintf("http://%s.%s.%s.svc.cluster.local:%d", pod.Name, pod.Spec.Subdomain, pod.Namespace, port), AgentType: agentType, Running: running, Ready: ready && eligible && running, Eligible: eligible}, pod, nil
}
func (k *KubernetesIdentity) Terminated(ctx context.Context, v d.Instance) (bool, error) {
	pod, err := k.API.CoreV1().Pods(v.Namespace).Get(ctx, v.ResourceName, metav1.GetOptions{})
	if err != nil {
		return false, ErrNativeUnavailable
	}
	if string(pod.UID) != v.ResourceUID {
		return false, nil
	} // Missing/replaced objects do not prove a stopped process.
	containerID := strings.SplitN(v.ResourceGeneration, "@", 2)[0]
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "agent" {
			for _, terminal := range []*core.ContainerStateTerminated{status.State.Terminated, status.LastTerminationState.Terminated} {
				if terminal != nil && terminal.ContainerID == containerID && !terminal.FinishedAt.IsZero() {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

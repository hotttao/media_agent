package main

import (
	"context"
	"encoding/json"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	controller "media_agent/media_runtime_controller/kubernetes"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	namespace := os.Getenv("RUNTIME_NAMESPACE")
	if namespace == "" {
		log.Fatal("RUNTIME_NAMESPACE required")
	}
	images := map[string]string{}
	if json.Unmarshal([]byte(os.Getenv("RUNTIME_ALLOWED_IMAGES")), &images) != nil || len(images) == 0 {
		log.Fatal("RUNTIME_ALLOWED_IMAGES map required")
	}
	var cfg *rest.Config
	var e error
	if file := os.Getenv("KUBECONFIG"); file != "" {
		cfg, e = clientcmd.BuildConfigFromFlags("", file)
	} else {
		cfg, e = rest.InClusterConfig()
	}
	if e != nil {
		log.Fatal("Kubernetes credentials unavailable")
	}
	cfg.Timeout = 5 * time.Second
	api, e := kubernetes.NewForConfig(cfg)
	if e != nil {
		log.Fatal("Kubernetes client unavailable")
	}
	dyn, e := dynamic.NewForConfig(cfg)
	if e != nil {
		log.Fatal("Kubernetes dynamic client unavailable")
	}
	reconciler := controller.Reconciler{API: api, Policy: controller.Policy{Namespace: namespace, SharedSecret: os.Getenv("RUNTIME_SHARED_SECRET"), ServiceAccount: "media-agent-runtime", Images: images}}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	reconciler.Run(ctx, dyn)
}

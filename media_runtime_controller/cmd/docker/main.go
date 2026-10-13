package main

import (
	"context"
	"encoding/json"
	"github.com/cloudwego/hertz/pkg/app/server"
	"log"
	containers "media_agent/media_runtime_controller/biz/handler/container_workload"
	identity "media_agent/media_runtime_controller/biz/handler/runtime_instance_identity"
	"media_agent/media_runtime_controller/biz/middleware"
	"media_agent/media_runtime_controller/biz/router"
	native "media_agent/media_runtime_controller/docker"
	"os"
)

func main() {
	var policy native.Policy
	file, e := os.ReadFile(os.Getenv("RUNTIME_DOCKER_POLICY_FILE"))
	if e != nil || json.Unmarshal(file, &policy) != nil || policy.EnvironmentID == "" || policy.DatabaseURL == "" || policy.RabbitURL == "" {
		log.Fatal("invalid private Docker runtime policy")
	}
	credential := os.Getenv("RUNTIME_CREATION_CREDENTIAL")
	if credential == "" {
		log.Fatal("runtime creation credential required")
	}
	socket := os.Getenv("RUNTIME_DOCKER_SOCKET")
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	engine := native.NewEngine(socket)
	if e = engine.CheckVersion(context.Background()); e != nil {
		log.Fatal("compatible Docker Engine API unavailable")
	}
	svc := &native.Service{Engine: engine, Policy: policy}
	middleware.Configure(credential)
	containers.Configure(svc)
	identity.Configure(svc)
	addr := os.Getenv("RUNTIME_CREATION_ADDRESS")
	if addr == "" {
		addr = ":8010"
	}
	h := server.Default(server.WithHostPorts(addr))
	router.GeneratedRegister(h)
	h.Spin()
}

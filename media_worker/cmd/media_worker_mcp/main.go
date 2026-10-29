// media_worker_mcp is a standalone management adapter. It never starts a browser
// or polls the execution gateway.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/hertz_infra/config"
	"media_agent/media_worker/biz/client/business_http"
	"media_agent/media_worker/biz/client/materials_api"
	"media_agent/media_worker/biz/client/workflow_api"
	handler "media_agent/media_worker/biz/handler/mcp"
	flow "media_agent/media_worker/biz/workflow/media_management"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	path := flag.String("config", "conf/mcp.yaml", "typed MCP server configuration")
	flag.Parse()
	if err := run(*path); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run(path string) error {
	loader, err := config.NewLoaderFromPath(path)
	if err != nil {
		return errors.New("cannot load MCP configuration")
	}
	defer loader.Close()
	cfg := loader.Current()
	d, m := cfg.GetMcpDelegation(), cfg.GetMediaMcp()
	if !d.GetEnabled() || !m.GetEnabled() || d.GetIssuer() == "" || d.GetJwksUrl() == "" || cfg.GetServer().GetAddress() == "" {
		return errors.New("MCP server and identity configuration required")
	}
	workflowClient, err := business_http.New(cfg.GetClient()["media_workflow"].GetBaseDomain(), d.GetWorkflowServiceToken(), mcpdelegation.Workflow)
	if err != nil {
		return err
	}
	materialsClient, err := business_http.New(cfg.GetClient()["media_materials"].GetBaseDomain(), d.GetMaterialsServiceToken(), mcpdelegation.Materials)
	if err != nil {
		return err
	}
	operations := flow.Operations{API: workflow_api.Client{HTTP: workflowClient}, Objects: materials_api.Client{HTTP: materialsClient}}
	endpoint, err := handler.New(flow.Management{API: operations}, internaljwt.NewVerifier(d.GetJwksUrl(), d.GetIssuer(), mcpdelegation.Backend, nil), internaljwt.NewVerifier(d.GetJwksUrl(), d.GetIssuer(), mcpdelegation.Workflow, nil), internaljwt.NewVerifier(d.GetJwksUrl(), d.GetIssuer(), mcpdelegation.Materials, nil))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			allowed := false
			for _, value := range m.GetAllowedOrigins() {
				if origin == value {
					allowed = true
					break
				}
			}
			if !allowed {
				http.Error(w, "Origin denied", 403)
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		endpoint.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	server := &http.Server{Addr: cfg.GetServer().GetAddress(), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 64 << 10}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(stop)
	}
}

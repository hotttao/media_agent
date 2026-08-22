package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_agent_gateway/biz/dal"
	repo "media_agent/media_agent_gateway/biz/dal/repo/agent_session"
	chathandler "media_agent/media_agent_gateway/biz/handler/chat"
	"media_agent/media_agent_gateway/biz/middleware"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
	sharedclient "media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
	"media_agent/media_agent_gateway/biz/workflow"
)

func main() {
	loader, err := config.NewLoader()
	if err != nil {
		log.Fatalf("init config loader: %v", err)
	}
	defer loader.Close()
	cfg := loader.Current()

	logCfg := cfg.GetLog()
	if logCfg.GetDir() == "" {
		logCfg.Dir = resolveDefaultLogDir(cfg.GetApp().GetName())
	}

	suite := serverhertz.NewServerSuite(loader)

	if err := loader.Attach(suite.KVSource()); err != nil {
		log.Fatalf("attach kv source: %v", err)
	}
	if err := loader.Watch(); err != nil {
		log.Fatalf("start config watch: %v", err)
	}

	shutdownObs, err := suite.InitObservability()
	if err != nil {
		log.Fatalf("init observability: %v", err)
	}
	defer shutdownObs(context.Background())

	dbCfg := cfg.GetDatabase()
	db, err := dal.NewDB(dbCfg)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	if err := dal.Migrate("migrations", dbCfg); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	if db == nil {
		log.Fatal("database must be enabled")
	}

	middleware.Configure(cfg.GetAuth().GetJwtSecret())
	store := repo.New(db)
	fallbackControl := sharedclient.MockControl{
		WorkspaceID: envOr("DEFAULT_WORKSPACE_ID", "default"),
		AgentID:     envOr("DEFAULT_AGENT_ID", "codex"),
	}
	control := &sharedclient.Control{BaseURL: envOr("MEDIA_CONTROL_URL", "http://127.0.0.1:8007"), ServiceToken: envOr("CONTROL_INTERNAL_TOKEN", ""), Fallback: fallbackControl}
	sessionService := service.New(store, dal.NewTxRunner(db), control)
	eventHub := sessionevents.New(256)
	turnWorkflow := workflow.NewRunTurn(sessionService, sharedclient.NewRuntimeRegistryFromEnvironment(), eventHub)
	chathandler.Configure(sessionService, turnWorkflow, eventHub)

	h := suite.NewServer()
	register(h)
	suite.RegisterRoutes(h)

	if err := suite.RegisterService(); err != nil {
		log.Fatalf("register service: %v", err)
	}
	defer func() {
		if err := suite.DeregisterService(); err != nil {
			log.Printf("deregister service: %v", err)
		}
	}()

	log.Printf("media-agent-gateway: listening on %s", cfg.GetServer().GetAddress())
	h.Spin()
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func resolveDefaultLogDir(appName string) string {
	wd, _ := os.Getwd()
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return filepath.Join(dir, "media_logs", appName)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

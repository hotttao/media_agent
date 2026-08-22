package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_control/biz/dal"
	templaterepo "media_agent/media_control/biz/dal/repo/agent_template"
	credentialrepo "media_agent/media_control/biz/dal/repo/credential_reference"
	memoryrepo "media_agent/media_control/biz/dal/repo/memory_store"
	promptrepo "media_agent/media_control/biz/dal/repo/prompt"
	providerrepo "media_agent/media_control/biz/dal/repo/provider"
	runtimerepo "media_agent/media_control/biz/dal/repo/runtime_environment"
	schedulerepo "media_agent/media_control/biz/dal/repo/schedule"
	skillrepo "media_agent/media_control/biz/dal/repo/skill"
	teamworkrepo "media_agent/media_control/biz/dal/repo/teamwork"
	repo "media_agent/media_control/biz/dal/repo/workspace"
	metadaterepo "media_agent/media_control/biz/dal/repo/workspace_metadata"
	templatehandler "media_agent/media_control/biz/handler/agent_template"
	credentialhandler "media_agent/media_control/biz/handler/credential_reference"
	memoryhandler "media_agent/media_control/biz/handler/memory_store"
	prompthandler "media_agent/media_control/biz/handler/prompt"
	providerhandler "media_agent/media_control/biz/handler/provider"
	runtimehandler "media_agent/media_control/biz/handler/runtime_environment"
	schedulehandler "media_agent/media_control/biz/handler/schedule"
	skillhandler "media_agent/media_control/biz/handler/skill"
	teamworkhandler "media_agent/media_control/biz/handler/teamwork"
	workspacehandler "media_agent/media_control/biz/handler/workspace"
	metadatahandler "media_agent/media_control/biz/handler/workspace_metadata"
	"media_agent/media_control/biz/middleware"
	templateservice "media_agent/media_control/biz/service/agent_template"
	credentialservice "media_agent/media_control/biz/service/credential_reference"
	memoryservice "media_agent/media_control/biz/service/memory_store"
	promptservice "media_agent/media_control/biz/service/prompt"
	providerservice "media_agent/media_control/biz/service/provider"
	runtimeservice "media_agent/media_control/biz/service/runtime_environment"
	scheduleservice "media_agent/media_control/biz/service/schedule"
	skillservice "media_agent/media_control/biz/service/skill"
	teamworkservice "media_agent/media_control/biz/service/teamwork"
	workspaceservice "media_agent/media_control/biz/service/workspace"
	metadataservice "media_agent/media_control/biz/service/workspace_metadata"
	"media_agent/media_control/biz/shared/secret"
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
	workspacehandler.Configure(
		workspaceservice.New(repo.New(db), dal.NewTxRunner(db), secret.New(envOr("CONTROL_SECRET_KEY", cfg.GetAuth().GetJwtSecret()))),
		envOr("CONTROL_INTERNAL_TOKEN", ""),
	)
	prompthandler.Configure(promptservice.New(promptrepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	templatehandler.Configure(templateservice.New(templaterepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	skillhandler.Configure(skillservice.New(skillrepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	providerhandler.Configure(providerservice.New(providerrepo.New(db), dal.NewTxRunner(db)))
	credentialhandler.Configure(credentialservice.New(credentialrepo.New(db), dal.NewTxRunner(db), repo.New(db), providerrepo.New(db)))
	memoryhandler.Configure(memoryservice.New(memoryrepo.New(db), dal.NewTxRunner(db), repo.New(db), credentialrepo.New(db)))
	schedulehandler.Configure(scheduleservice.New(schedulerepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	runtimehandler.Configure(runtimeservice.New(runtimerepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	teamworkhandler.Configure(teamworkservice.New(teamworkrepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	metadatahandler.Configure(metadataservice.New(metadaterepo.New(db), dal.NewTxRunner(db), repo.New(db)))

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

	log.Printf("media-control: listening on %s", cfg.GetServer().GetAddress())
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

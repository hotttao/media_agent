package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_agent_gateway/biz/dal"
	repo "media_agent/media_agent_gateway/biz/dal/repo/agent_session"
	taskrepo "media_agent/media_agent_gateway/biz/dal/repo/agent_task"
	batchrepo "media_agent/media_agent_gateway/biz/dal/repo/batch_run"
	teamworkrepo "media_agent/media_agent_gateway/biz/dal/repo/teamwork_runtime"
	agentclienthandler "media_agent/media_agent_gateway/biz/handler/agent_client"
	agentproxyhandler "media_agent/media_agent_gateway/biz/handler/agent_proxy"
	taskhandler "media_agent/media_agent_gateway/biz/handler/agent_task"
	batchhandler "media_agent/media_agent_gateway/biz/handler/batch_run"
	chathandler "media_agent/media_agent_gateway/biz/handler/chat"
	teamworkhandler "media_agent/media_agent_gateway/biz/handler/teamwork_runtime"
	"media_agent/media_agent_gateway/biz/middleware"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
	taskservice "media_agent/media_agent_gateway/biz/service/agent_task"
	batchservice "media_agent/media_agent_gateway/biz/service/batch_run"
	sharedclient "media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/shared/riverqueue"
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
	"media_agent/media_agent_gateway/biz/workflow"
	taskdispatch "media_agent/media_agent_gateway/biz/workflow/agent_task_dispatch"
	batchdispatch "media_agent/media_agent_gateway/biz/workflow/batch_run_dispatch"
	teamworkchange "media_agent/media_agent_gateway/biz/workflow/teamwork_runtime_change"
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

	internalToken := envOr("CONTROL_INTERNAL_TOKEN", "")
	middleware.Configure(cfg.GetAuth().GetJwtSecret(), internalToken)
	if credential := os.Getenv("MEDIA_AUTH_AGENT_GATEWAY_TOKEN"); credential != "" {
		agentTokens, err := sharedclient.NewAgentTokenClient(os.Getenv("MEDIA_AUTH_URL"), os.Getenv("MEDIA_CONTROL_URL"), internalToken, credential, internaljwt.NewVerifier(os.Getenv("INTERNAL_JWKS_URL"), os.Getenv("INTERNAL_JWT_ISSUER"), "internal-api", nil))
		if err != nil || os.Getenv("INTERNAL_JWKS_URL") == "" || os.Getenv("INTERNAL_JWT_ISSUER") == "" {
			log.Fatal("invalid Agent Token management configuration")
		}
		middleware.ConfigureAgentTokenManagement(agentTokens)
		agentclienthandler.Configure(workflow.NewAgentTokenProvisioning(agentTokens, agentTokens))
		if keyFile := os.Getenv("AGENT_SIGNING_KEY_FILE"); keyFile != "" {
			signer, err := internaljwt.LoadAgentSigner(keyFile, os.Getenv("AGENT_JWT_ISSUER"))
			if err != nil {
				log.Fatal("invalid Agent signing configuration")
			}
			credentials, err := sharedclient.NewAgentCredentialClient(os.Getenv("MEDIA_AUTH_URL"), os.Getenv("MEDIA_AUTH_INTERNAL_TOKEN"))
			if err != nil {
				log.Fatal("invalid Agent credential verification configuration")
			}
			forwarder, err := sharedclient.NewAgentHTTPForwarder(os.Getenv("MEDIA_WORKFLOW_URL"), os.Getenv("MEDIA_MATERIALS_URL"))
			if err != nil {
				log.Fatal("invalid Agent forwarding configuration")
			}
			agentproxyhandler.Configure(workflow.NewAgentAPIProxy(credentials, agentTokens, signer, forwarder), signer)
		}
	} else if os.Getenv("AGENT_SIGNING_KEY_FILE") != "" {
		log.Fatal("Agent proxy requires Agent authority configuration")
	}
	store := repo.New(db)
	fallbackControl := sharedclient.MockControl{
		WorkspaceID: envOr("DEFAULT_WORKSPACE_ID", "default"),
		AgentID:     envOr("DEFAULT_AGENT_ID", "codex"),
	}
	control := &sharedclient.Control{BaseURL: envOr("MEDIA_CONTROL_URL", "http://127.0.0.1:8007"), ServiceToken: envOr("CONTROL_INTERNAL_TOKEN", ""), Fallback: fallbackControl}
	sessionService := service.New(store, control)
	taskStore := taskrepo.New(db)
	batchStore := batchrepo.New(db)
	teamworkStore := teamworkrepo.New(db)
	txRunner := dal.NewTxRunner(db)
	runtimes := sharedclient.NewRuntimeRegistryFromEnvironment()
	taskApplication := taskservice.New(taskStore, taskStore, txRunner, store, control, teamworkStore)
	batchApplication := batchservice.New(batchStore, batchStore, txRunner, store)
	taskDispatcher := taskdispatch.NewDispatcher(taskStore, taskStore, txRunner, sessionService, runtimes)
	batchDispatcher := batchdispatch.NewDispatcher(batchStore, txRunner, taskApplication)
	dispatchCtx, cancelDispatch := context.WithCancel(context.Background())
	taskQueue, err := riverqueue.New(dispatchCtx, dbCfg.GetDsn(), dbCfg.GetMigrateOnStart(), riverqueue.Handlers{AgentTask: taskDispatcher.Process, BatchRun: batchDispatcher.Process})
	if err != nil {
		log.Fatalf("init Agent Task River queue: %v", err)
	}
	if err := taskQueue.Start(dispatchCtx); err != nil {
		log.Fatalf("start Agent Task River queue: %v", err)
	}
	defer func() {
		cancelDispatch()
		if err := taskQueue.Close(); err != nil {
			log.Printf("stop Agent Task River queue: %v", err)
		}
	}()
	go taskdispatch.NewLoops(taskdispatch.NewRelay(taskStore, taskQueue), taskDispatcher, time.Second).Run(dispatchCtx)
	go batchdispatch.NewLoops(batchdispatch.NewRelay(batchStore, taskQueue), batchdispatch.NewProjector(taskStore, batchStore, txRunner), time.Second).Run(dispatchCtx)
	taskhandler.Configure(taskApplication)
	batchhandler.Configure(batchApplication)
	teamworkhandler.Configure(teamworkchange.New(teamworkStore, taskStore, taskStore, txRunner))
	var eventSource sessionevents.Source = sessionevents.New(256)
	if redisURL := os.Getenv("MEDIA_AGENTS_REDIS_URL"); redisURL != "" {
		redisSource, redisErr := sessionevents.NewRedisSource(
			redisURL,
			envOr("MEDIA_AGENTS_EVENT_CHANNEL_PREFIX", "media-agents:session-events"),
			256,
		)
		if redisErr != nil {
			log.Fatalf("init Session Event Bus: %v", redisErr)
		}
		eventSource = redisSource
	}
	defer eventSource.Close()
	turnWorkflow := workflow.NewRunTurn(sessionService, runtimes)
	chathandler.Configure(sessionService, turnWorkflow, eventSource)

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

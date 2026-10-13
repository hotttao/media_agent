package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/mcpprotocol"
	"media_agent/hertz_infra/messagebus"
	"media_agent/hertz_infra/runtimeauth"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/hertz_infra/sessioncredential"
	"media_agent/media_agent_gateway/biz/dal"
	instancerepo "media_agent/media_agent_gateway/biz/dal/repo/agent_instance"
	repo "media_agent/media_agent_gateway/biz/dal/repo/agent_session"
	taskrepo "media_agent/media_agent_gateway/biz/dal/repo/agent_task"
	batchrepo "media_agent/media_agent_gateway/biz/dal/repo/batch_run"
	catalogrepo "media_agent/media_agent_gateway/biz/dal/repo/mcp_catalog"
	teamworkrepo "media_agent/media_agent_gateway/biz/dal/repo/teamwork_runtime"
	instancedomain "media_agent/media_agent_gateway/biz/domain/agent_instance"
	agentclienthandler "media_agent/media_agent_gateway/biz/handler/agent_client"
	metadatahandler "media_agent/media_agent_gateway/biz/handler/agent_metadata"
	agentproxyhandler "media_agent/media_agent_gateway/biz/handler/agent_proxy"
	runtimehandler "media_agent/media_agent_gateway/biz/handler/agent_runtime"
	taskhandler "media_agent/media_agent_gateway/biz/handler/agent_task"
	workloadhandler "media_agent/media_agent_gateway/biz/handler/agent_workload"
	batchhandler "media_agent/media_agent_gateway/biz/handler/batch_run"
	chathandler "media_agent/media_agent_gateway/biz/handler/chat"
	cataloghandler "media_agent/media_agent_gateway/biz/handler/mcp_catalog"
	teamworkhandler "media_agent/media_agent_gateway/biz/handler/teamwork_runtime"
	"media_agent/media_agent_gateway/biz/middleware"
	instanceservice "media_agent/media_agent_gateway/biz/service/agent_instance"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
	taskservice "media_agent/media_agent_gateway/biz/service/agent_task"
	batchservice "media_agent/media_agent_gateway/biz/service/batch_run"
	catalogservice "media_agent/media_agent_gateway/biz/service/mcp_catalog"
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
	var agentAuthority *sharedclient.AgentTokenClient
	var credentialVerifier *sharedclient.AgentCredentialClient
	var metadataFlow *workflow.AgentMetadata
	if credential := os.Getenv("MEDIA_AUTH_AGENT_GATEWAY_TOKEN"); credential != "" {
		agentTokens, err := sharedclient.NewAgentTokenClient(os.Getenv("MEDIA_AUTH_URL"), os.Getenv("MEDIA_CONTROL_URL"), internalToken, credential, internaljwt.NewVerifier(os.Getenv("INTERNAL_JWKS_URL"), os.Getenv("INTERNAL_JWT_ISSUER"), "internal-api", nil))
		if err != nil || os.Getenv("INTERNAL_JWKS_URL") == "" || os.Getenv("INTERNAL_JWT_ISSUER") == "" {
			log.Fatal("invalid Agent Token management configuration")
		}
		agentAuthority = agentTokens
		middleware.ConfigureAgentTokenManagement(agentTokens)
		if cfg.GetAgentMetadata().GetEnabled() {
			credentials, err := sharedclient.NewAgentCredentialClient(os.Getenv("MEDIA_AUTH_URL"), os.Getenv("MEDIA_AUTH_INTERNAL_TOKEN"))
			if err != nil {
				log.Fatal("invalid metadata Agent authentication")
			}
			credentialVerifier = credentials
			metadataFlow = workflow.NewAgentMetadata(credentials, agentTokens, instancerepo.New(db), sharedclient.NewMetadataControlClient(os.Getenv("MEDIA_CONTROL_URL"), internalToken, cfg.GetAgentMetadata().GetGatewayCredential()))
			metadatahandler.Configure(metadataFlow)
		}

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
	if cfg.GetAgentWorkload().GetEnabled() && !cfg.GetAgentRuntime().GetEnabled() {
		log.Fatal("workload provisioning requires managed Agent Runtime")
	}
	var provisioning *workflow.WorkloadProvisioning
	var routing *workflow.SessionRouting
	var execution *workflow.ExecutionContext
	var credentialKeys *sessioncredential.Ring
	if runtimeCfg := cfg.GetAgentRuntime(); runtimeCfg.GetEnabled() {
		if agentAuthority == nil || credentialVerifier == nil || metadataFlow == nil || runtimeCfg.GetEnvironmentId() == "" {
			log.Fatal("Agent Runtime requires metadata, Agent authority and environment configuration")
		}
		keys, err := sessioncredential.Load(runtimeCfg.GetCredentialKeysFile())
		if err != nil {
			log.Fatal("invalid Agent Runtime credential keys")
		}
		signer, err := runtimeauth.Load(runtimeCfg.GetSigningKeyFile())
		if err != nil {
			log.Fatal("invalid Agent Runtime signing key")
		}
		credentialKeys = keys
		sessionService.WithCredentials(sharedclient.SessionCredentials{Keys: keys})
		var native instancedomain.ResourceAuthority
		if runtimeCfg.GetDockerIdentityUrl() != "" {
			if runtimeCfg.GetDockerInternalCredential() == "" {
				log.Fatal("Docker identity service credential required")
			}
			native = sharedclient.DockerIdentity{EnvironmentID: runtimeCfg.GetEnvironmentId(), BaseURL: runtimeCfg.GetDockerIdentityUrl(), ServiceToken: runtimeCfg.GetDockerInternalCredential()}
		} else {
			native, err = sharedclient.NewKubernetesIdentity(runtimeCfg.GetEnvironmentId(), runtimeCfg.GetKubernetesNamespace(), runtimeCfg.GetKubernetesConfigFile())
			if err != nil {
				log.Fatal("invalid Kubernetes identity configuration")
			}
		}
		placement := sharedclient.PlacementAuthority{BaseURL: os.Getenv("MEDIA_CONTROL_URL"), ServiceToken: internalToken}
		instances := instanceservice.New(instancerepo.NewRegistration(db), txRunner)
		registration := workflow.NewInstanceRegistration(credentialVerifier, agentAuthority, placement, native, instances, keys)
		routing = workflow.NewSessionRouting(store, sessionService, instances, placement, native, sharedclient.ScopedRuntimeFactory{Signer: signer})
		execution = workflow.NewExecutionContext(registration, store, sessionService, routing)
		runtimehandler.Configure(registration, execution)

		if cfg.GetAgentWorkload().GetEnabled() {
			var resources workflow.WorkloadResources
			if kube, ok := native.(*sharedclient.KubernetesIdentity); ok {
				resources = sharedclient.KubernetesWorkloads{API: kube.Dynamic, Native: kube.API, EnvironmentID: kube.EnvironmentID, Namespace: kube.Namespace}
			} else {
				resources = sharedclient.DockerWorkloads{EnvironmentID: runtimeCfg.GetEnvironmentId(), BaseURL: runtimeCfg.GetDockerIdentityUrl(), ServiceToken: runtimeCfg.GetDockerInternalCredential()}
			}
			provisioning = &workflow.WorkloadProvisioning{Control: sharedclient.WorkloadControl{BaseURL: control.BaseURL, ServiceToken: internalToken}, Resources: resources, EnvironmentID: runtimeCfg.GetEnvironmentId(), Instances: instances, Bindings: store, Routing: routing, Factory: sharedclient.ScopedRuntimeFactory{Signer: signer}, Native: native}
			workloadhandler.Configure(provisioning)
			routing.WithProvisioner(provisioning)
			metadataFlow.WithMembership(provisioning)
		}
		metadataFlow.WithNativeGuard(routing)
	}

	catalogApplication := catalogservice.New(catalogrepo.New(db), txRunner, cfg.GetAgentMcp().GetAllowedUpstreamHosts())
	cataloghandler.Configure(catalogApplication, nil)
	if mcpCfg := cfg.GetAgentMcp(); mcpCfg.GetEnabled() {
		if execution == nil || credentialKeys == nil {
			log.Fatal("MCP requires Agent Runtime identity configuration")
		}
		var access workflow.MCPBindingAccess
		if mcpCfg.GetPluginUrl() != "" {
			if mcpCfg.GetPluginCredential() == "" {
				log.Fatal("MCP Plugin service credential required")
			}
			access = sharedclient.PluginMCPAccess{BaseURL: mcpCfg.GetPluginUrl(), Credential: mcpCfg.GetPluginCredential()}
		}
		identity := workflow.NewMCPIdentity(sessionService, credentialVerifier, agentAuthority, execution)
		gateway := workflow.NewMCPGateway(identity, catalogApplication, access, sharedclient.MCPUpstream{}, credentialKeys, sharedclient.NewMetadataControlClient(os.Getenv("MEDIA_CONTROL_URL"), internalToken, cfg.GetAgentMetadata().GetGatewayCredential()))
		cataloghandler.Configure(catalogApplication, mcpprotocol.NewHandler(cataloghandler.ProtocolFactory(identity, gateway)))
	}

	taskApplication := taskservice.New(taskStore, taskStore, txRunner, store, control, teamworkStore)
	batchApplication := batchservice.New(batchStore, batchStore, txRunner, store)
	taskDispatcher := taskdispatch.NewDispatcher(taskStore, taskStore, txRunner, sessionService, runtimes)
	if routing != nil {
		taskDispatcher.WithRouting(routing)
	}
	batchDispatcher := batchdispatch.NewDispatcher(batchStore, txRunner, taskApplication)
	dispatchCtx, cancelDispatch := context.WithCancel(context.Background())
	if cfg.GetAgentMcp().GetEnabled() && cfg.GetRabbitmq().GetEnabled() {
		publisher, e := messagebus.NewTopicPublisher(cfg.GetRabbitmq().GetUrl(), "media.mcp.catalog")
		if e != nil {
			log.Fatal("invalid MCP catalog event configuration")
		}
		defer publisher.Close()
		go workflow.RelayMCPCatalog(dispatchCtx, catalogApplication, publisher)
	}
	if routing != nil {
		go routing.Run(dispatchCtx)
	}
	if provisioning != nil {
		go provisioning.Run(dispatchCtx)
	}
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
	if busCfg := cfg.GetRabbitmq(); busCfg.GetEnabled() {
		rabbitSource, busErr := sessionevents.NewRabbitSource(
			busCfg.GetUrl(),
			busCfg.GetSessionExchange(),
			int(busCfg.GetReceiverBuffer()),
		)
		if busErr != nil {
			log.Fatalf("init Session Event Bus: %v", busErr)
		}
		eventSource = rabbitSource
	}
	defer eventSource.Close()
	turnWorkflow := workflow.NewRunTurn(sessionService, runtimes)
	if routing != nil {
		turnWorkflow.WithRouting(routing)
	}
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

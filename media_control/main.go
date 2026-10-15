package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/resourceaccess"
	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/messagebus"
	"media_agent/hertz_infra/metadataartifact"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_control/biz/dal"
	afsrepo "media_agent/media_control/biz/dal/repo/afs_binding"
	metadatarepo "media_agent/media_control/biz/dal/repo/agent_configuration"
	templaterepo "media_agent/media_control/biz/dal/repo/agent_template"
	credentialrepo "media_agent/media_control/biz/dal/repo/credential_reference"
	grantrepo "media_agent/media_control/biz/dal/repo/environment_grant"
	memoryrepo "media_agent/media_control/biz/dal/repo/memory_store"
	adminrepo "media_agent/media_control/biz/dal/repo/platform_administration"
	promptrepo "media_agent/media_control/biz/dal/repo/prompt"
	providerrepo "media_agent/media_control/biz/dal/repo/provider"
	exportrepo "media_agent/media_control/biz/dal/repo/public_export"
	runtimerepo "media_agent/media_control/biz/dal/repo/runtime_environment"
	schedulerepo "media_agent/media_control/biz/dal/repo/schedule"
	sharerepo "media_agent/media_control/biz/dal/repo/share_metadata"
	skillrepo "media_agent/media_control/biz/dal/repo/skill"
	teamworkrepo "media_agent/media_control/biz/dal/repo/teamwork"
	repo "media_agent/media_control/biz/dal/repo/workspace"
	metadaterepo "media_agent/media_control/biz/dal/repo/workspace_metadata"
	afshandler "media_agent/media_control/biz/handler/afs_binding"
	agentidentityhandler "media_agent/media_control/biz/handler/agent_identity"
	metadatahandler "media_agent/media_control/biz/handler/agent_metadata"
	templatehandler "media_agent/media_control/biz/handler/agent_template"
	workloadhandler "media_agent/media_control/biz/handler/agent_workload"
	credentialhandler "media_agent/media_control/biz/handler/credential_reference"
	granthandler "media_agent/media_control/biz/handler/environment_grant"
	memoryhandler "media_agent/media_control/biz/handler/memory_store"
	adminhandler "media_agent/media_control/biz/handler/platform_administration"
	prompthandler "media_agent/media_control/biz/handler/prompt"
	providerhandler "media_agent/media_control/biz/handler/provider"
	exporthandler "media_agent/media_control/biz/handler/public_export"
	runtimehandler "media_agent/media_control/biz/handler/runtime_environment"
	schedulehandler "media_agent/media_control/biz/handler/schedule"
	sharehandler "media_agent/media_control/biz/handler/share_metadata"
	skillhandler "media_agent/media_control/biz/handler/skill"
	teamworkhandler "media_agent/media_control/biz/handler/teamwork"
	workspacehandler "media_agent/media_control/biz/handler/workspace"
	workspacemetadatahandler "media_agent/media_control/biz/handler/workspace_metadata"
	"media_agent/media_control/biz/middleware"
	afsservice "media_agent/media_control/biz/service/afs_binding"
	metadataservice "media_agent/media_control/biz/service/agent_configuration"
	agentidentityservice "media_agent/media_control/biz/service/agent_identity"
	templateservice "media_agent/media_control/biz/service/agent_template"
	credentialservice "media_agent/media_control/biz/service/credential_reference"
	grantservice "media_agent/media_control/biz/service/environment_grant"
	memoryservice "media_agent/media_control/biz/service/memory_store"
	adminservice "media_agent/media_control/biz/service/platform_administration"
	promptservice "media_agent/media_control/biz/service/prompt"
	providerservice "media_agent/media_control/biz/service/provider"
	exportservice "media_agent/media_control/biz/service/public_export"
	runtimeservice "media_agent/media_control/biz/service/runtime_environment"
	scheduleservice "media_agent/media_control/biz/service/schedule"
	shareservice "media_agent/media_control/biz/service/share_metadata"
	skillservice "media_agent/media_control/biz/service/skill"
	teamworkservice "media_agent/media_control/biz/service/teamwork"
	workspaceservice "media_agent/media_control/biz/service/workspace"
	workspacemetadataservice "media_agent/media_control/biz/service/workspace_metadata"
	sharedclient "media_agent/media_control/biz/shared/client"
	mediaauthclient "media_agent/media_control/biz/shared/client/mediaauth"
	"media_agent/media_control/biz/shared/client/skillgit"
	"media_agent/media_control/biz/shared/secret"
	metadataflow "media_agent/media_control/biz/workflow/metadata_update"
	skillflow "media_agent/media_control/biz/workflow/skill_content"
	teamworkchange "media_agent/media_control/biz/workflow/teamwork_change"
	workloadflow "media_agent/media_control/biz/workflow/workload_target"
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
	mediaAuthURL := envOr("MEDIA_AUTH_URL", "http://127.0.0.1:8001")
	mediaAuthInternalToken := envOr("MEDIA_AUTH_INTERNAL_TOKEN", "")
	if jwksURL := os.Getenv("INTERNAL_JWKS_URL"); jwksURL != "" {
		middleware.ConfigureOry(internaljwt.NewVerifier(jwksURL, os.Getenv("INTERNAL_JWT_ISSUER"), "internal-api", nil), os.Getenv("ORY_KRATOS_PUBLIC_URL"), mediaAuthURL, mediaAuthInternalToken)
	}
	teamAccess := mediaauthclient.New(mediaAuthURL, mediaAuthInternalToken, nil)
	agentidentityhandler.Configure(agentidentityservice.New(repo.New(db), teamAccess))
	workspaceApplication := workspaceservice.New(repo.New(db), dal.NewTxRunner(db), secret.New(envOr("CONTROL_SECRET_KEY", cfg.GetAuth().GetJwtSecret())), teamAccess)
	workspacehandler.Configure(workspaceApplication, internalToken)
	promptApplication := promptservice.New(promptrepo.New(db), dal.NewTxRunner(db), resourceaccess.New(mediaAuthURL, mediaAuthInternalToken, nil))
	prompthandler.Configure(promptApplication)
	templatehandler.Configure(templateservice.New(templaterepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	artifactRoot := cfg.GetAgentMetadata().GetArtifactRoot()
	if artifactRoot == "" {
		artifactRoot = envOr("AGENT_METADATA_ARTIFACT_ROOT", "./data/metadata")
	}
	artifacts, err := metadataartifact.New(artifactRoot)
	if err != nil {
		log.Fatal("invalid metadata artifact root")
	}
	skillApplication := skillservice.New(skillrepo.New(db), dal.NewTxRunner(db), resourceaccess.New(mediaAuthURL, mediaAuthInternalToken, nil))
	skillContent := skillflow.New(skillApplication, artifacts, skillgit.Client{}, secret.New(envOr("CONTROL_SECRET_KEY", cfg.GetAuth().GetJwtSecret())))
	skillhandler.Configure(skillContent)
	providerhandler.Configure(providerservice.New(providerrepo.New(db), dal.NewTxRunner(db)))
	adminStore := adminrepo.New(db)
	adminhandler.Configure(adminservice.New(adminStore, dal.NewTxRunner(db)), adminservice.NewClusterService(adminStore), adminservice.NewStatisticsService(adminStore), adminservice.NewAdminWorkspaceService(adminStore))
	credentialhandler.Configure(credentialservice.New(credentialrepo.New(db), dal.NewTxRunner(db), repo.New(db), providerrepo.New(db)))
	memoryhandler.Configure(memoryservice.New(memoryrepo.New(db), dal.NewTxRunner(db), repo.New(db), credentialrepo.New(db)))
	schedulehandler.Configure(scheduleservice.New(schedulerepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	runtimeApplication := runtimeservice.New(runtimerepo.New(db), dal.NewTxRunner(db), repo.New(db))
	runtimehandler.Configure(runtimeApplication)
	if workloadCfg := cfg.GetAgentWorkload(); workloadCfg.GetEnabled() {
		if !cfg.GetAgentMetadata().GetEnabled() {
			log.Fatal("workload provisioning requires Agent Metadata")
		}
		if internalToken == "" {
			log.Fatal("workload Control service credential is required")
		}
		profiles := map[string]workloadflow.Profile{}
		for kind, profile := range workloadCfg.GetProfiles() {
			profiles[kind] = workloadflow.Profile{Image: profile.GetImage(), CPU: profile.GetCpu(), Memory: profile.GetMemory(), Port: profile.GetPort()}
		}
		workloadhandler.Configure(&workloadflow.Flow{Service: runtimeApplication, Workspaces: workspaceApplication, Policy: workloadflow.Policy{SessionExchange: workloadCfg.GetSessionExchange(), MetadataExchange: cfg.GetAgentMetadata().GetMetadataExchange(), Driver: workloadCfg.GetDriver(), Namespace: workloadCfg.GetNamespace(), SharedSecret: workloadCfg.GetSharedSecret(), StorageClass: workloadCfg.GetStorageClass(), StorageSize: workloadCfg.GetStorageSize(), GatewayURL: workloadCfg.GetGatewayUrl(), Profiles: profiles}})
	}
	teamworkStore := teamworkrepo.New(db)
	teamworkhandler.Configure(teamworkservice.New(teamworkStore, teamworkStore, dal.NewTxRunner(db), repo.New(db)))
	eventCtx, cancelEvents := context.WithCancel(context.Background())
	defer cancelEvents()
	teamworkPublisher, err := sharedclient.NewTeamworkEventPublisher(envOr("MEDIA_AGENT_GATEWAY_URL", "http://127.0.0.1:8006"), internalToken)
	if err != nil {
		log.Fatalf("init Teamwork event publisher: %v", err)
	}
	go teamworkchange.NewRelay(teamworkStore, teamworkPublisher).Run(eventCtx, time.Second)
	workspacemetadatahandler.Configure(workspacemetadataservice.New(metadaterepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	sharehandler.Configure(shareservice.New(sharerepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	exporthandler.Configure(exportservice.New(exportrepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	afshandler.Configure(afsservice.New(afsrepo.New(db), dal.NewTxRunner(db), repo.New(db)))
	granthandler.Configure(grantservice.New(grantrepo.New(db), dal.NewTxRunner(db)))

	if metadataCfg := cfg.GetAgentMetadata(); metadataCfg.GetEnabled() {
		if metadataCfg.GetGatewayCredential() == "" {
			log.Fatal("metadata Gateway identity credential is required")
		}
		middleware.ConfigureMetadataGateway(metadataCfg.GetGatewayCredential())
		repository := metadatarepo.New(db)
		workspaceAccess := workspaceservice.New(repo.New(db), dal.NewTxRunner(db), secret.New(envOr("CONTROL_SECRET_KEY", cfg.GetAuth().GetJwtSecret())), teamAccess)
		metadatahandler.Configure(metadataflow.New(metadataservice.New(repository, dal.NewTxRunner(db)), workspaceAccess, artifacts, sharedclient.NewMetadataInstances(envOr("MEDIA_AGENT_GATEWAY_URL", "http://127.0.0.1:8006"), internalToken)).WithPrompts(promptApplication).WithSkills(skillContent))
		publisher, err := messagebus.NewTopicPublisher(cfg.GetRabbitmq().GetUrl(), metadataCfg.GetMetadataExchange())
		if err != nil {
			log.Fatal("invalid metadata Broker configuration")
		}
		defer publisher.Close()
		go metadataflow.NewRelay(repository, publisher, dal.NewTxRunner(db)).Run(eventCtx)
	}
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

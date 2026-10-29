// main.go 是 media_workflow 程序入口。
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/hertz_infra/authn/resourceaccess"
	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_workflow/biz/dal"
	capabilityrepo "media_agent/media_workflow/biz/dal/repo/capability"
	historypushrepo "media_agent/media_workflow/biz/dal/repo/historical_publication"
	jobrepo "media_agent/media_workflow/biz/dal/repo/job"
	tplrepo "media_agent/media_workflow/biz/dal/repo/template"
	wfrepo "media_agent/media_workflow/biz/dal/repo/workflow"
	templatedomain "media_agent/media_workflow/biz/domain/template"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	capabilityhandler "media_agent/media_workflow/biz/handler/capability"
	historypushhandler "media_agent/media_workflow/biz/handler/historical_publication"
	jobhandler "media_agent/media_workflow/biz/handler/job"
	tplhandler "media_agent/media_workflow/biz/handler/template"
	workerhandler "media_agent/media_workflow/biz/handler/worker"
	wfhandler "media_agent/media_workflow/biz/handler/workflow"
	capabilitysvc "media_agent/media_workflow/biz/service/capability"
	historypushsvc "media_agent/media_workflow/biz/service/historical_publication"
	jobsvc "media_agent/media_workflow/biz/service/job"
	tplsvc "media_agent/media_workflow/biz/service/template"
	wfsvc "media_agent/media_workflow/biz/service/workflow"
	"media_agent/media_workflow/biz/shared"
	"media_agent/media_workflow/biz/shared/auth"
	"media_agent/media_workflow/biz/shared/client"
	mcpworkflow "media_agent/media_workflow/biz/workflow"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/publication"
	reviewdelivery "media_agent/media_workflow/biz/workflow/review_delivery"
	startdelivery "media_agent/media_workflow/biz/workflow/start_delivery"
	workergateway "media_agent/media_workflow/biz/workflow/worker_gateway"
)

// templateLookupAdapter 把 template.Service 适配为 workflow.TemplateLookup 端口。
type templateLookupAdapter struct{ svc *tplsvc.Service }

// Only execution use cases receive this immutable snapshot port. Job-level
// authorization precedes access; it is never used by definition handlers.
type executionTemplateLookupAdapter struct{ svc *tplsvc.Service }

func (a *executionTemplateLookupAdapter) GetVersion(ctx context.Context, id string, version int) (*wfdomain.TemplateSnapshot, error) {
	tv, err := a.svc.GetVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	slots := make([]wfdomain.SlotSnapshot, 0, len(tv.Inputs))
	for _, in := range tv.Inputs {
		slots = append(slots, wfdomain.SlotSnapshot{Name: in.Name, AssetKind: in.AssetKind, ResourceType: in.ResourceType, Required: in.Required})
	}
	return &wfdomain.TemplateSnapshot{ID: tv.TemplateID, Status: string(tv.Status), PromptBody: tv.PromptBody, OutputAssetKind: tv.OutputAssetKind, Platform: tv.Platform, GenWay: tv.GenWay, GenType: tv.GenType, NeedsConfirm: tv.NeedsConfirm, CandidateCount: tv.CandidateCount, CapabilityKey: tv.CapabilityKey, CapabilityVersion: tv.CapabilityVersion, InvocationMapping: invocationSnapshot(tv.InvocationMapping), Inputs: slots}, nil
}

func (a *templateLookupAdapter) GetVersion(ctx context.Context, id string, version int) (*wfdomain.TemplateSnapshot, error) {
	tv, err := a.svc.GetVersionForCaller(ctx, id, version, auth.CallerID(ctx))
	if err != nil {
		return nil, err
	}
	slots := make([]wfdomain.SlotSnapshot, len(tv.Inputs))
	for i, in := range tv.Inputs {
		slots[i] = wfdomain.SlotSnapshot{Name: in.Name, AssetKind: in.AssetKind, ResourceType: in.ResourceType, Required: in.Required}
	}
	// status 需从当前 template 取（version 快照无 status；查当前）
	t, err := a.svc.Get(ctx, id, auth.CallerID(ctx))
	if err != nil {
		return nil, err
	}
	versionStatus := tv.Status
	if t.Status == templatedomain.StatusArchived {
		versionStatus = templatedomain.StatusArchived
	}
	return &wfdomain.TemplateSnapshot{
		OrganizationID: t.OrganizationID, ID: tv.TemplateID, Scope: t.Scope, Status: string(versionStatus),
		OutputAssetKind: tv.OutputAssetKind, PromptBody: tv.PromptBody, Platform: tv.Platform,
		GenWay: tv.GenWay, GenType: tv.GenType,
		NeedsConfirm: tv.NeedsConfirm, CandidateCount: tv.CandidateCount, Inputs: slots,
		CapabilityKey: tv.CapabilityKey, CapabilityVersion: tv.CapabilityVersion,
		InvocationMapping: invocationSnapshot(tv.InvocationMapping),
	}, nil
}

func invocationSnapshot(in map[string]templatedomain.InvocationExpression) map[string]wfdomain.InvocationExpressionSnapshot {
	out := make(map[string]wfdomain.InvocationExpressionSnapshot, len(in))
	for name, expression := range in {
		out[name] = invocationExpressionSnapshot(expression)
	}
	return out
}

func invocationExpressionSnapshot(in templatedomain.InvocationExpression) wfdomain.InvocationExpressionSnapshot {
	out := wfdomain.InvocationExpressionSnapshot{Op: string(in.Op), Slot: in.Slot}
	for _, item := range in.Items {
		out.Items = append(out.Items, invocationExpressionSnapshot(item))
	}
	return out
}

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
	txRunner := dal.NewTxRunner(db)

	// materials client（调 media_materials.BatchGetAssetKind + UpsertGeneratedAsset）
	materialsBaseURL := "http://127.0.0.1:8003"
	if c, ok := cfg.GetClient()["media_materials"]; ok && c.GetBaseDomain() != "" {
		materialsBaseURL = c.GetBaseDomain()
	}
	materialsClient := client.NewMaterialsClient(materialsBaseURL)

	// template 装配
	capabilityRepo := capabilityrepo.NewRepo(db)
	capabilityhandler.Svc = capabilitysvc.NewService(capabilityRepo)
	tplRepo := tplrepo.NewRepo(db)
	tplGrantRepo := tplrepo.NewGrantRepo(db)
	tplSvc := tplsvc.NewService(tplRepo, tplGrantRepo, txRunner, materialsClient, capabilityRepo)
	tplhandler.Svc = tplSvc

	// workflow 装配（TemplateLookup 由 templateSvc 适配）
	tplLookup := &templateLookupAdapter{svc: tplSvc}
	wfRepo := wfrepo.NewRepo(db)
	wfGrantRepo := wfrepo.NewGrantRepo(db)
	compilationRepo := wfrepo.NewCompilationRepo(db)
	conductorClient := conductor.NewClient(os.Getenv("CONDUCTOR_URL"))
	publishFlow := &publication.Flow{
		Templates: tplLookup, Capabilities: capabilityRepo, AssetKinds: materialsClient, Intents: compilationRepo, Workflows: wfRepo, Tx: txRunner,
	}
	wfSvc := wfsvc.NewService(wfRepo, wfGrantRepo, txRunner, tplLookup, publishFlow)
	wfSvc.ConfigurePublications(compilationRepo)
	wfSvc.ConfigureVersions(wfRepo)
	wfhandler.Svc = wfSvc

	// job 装配（P3）
	jobRepo := jobrepo.NewJobRepo(db)
	stepRunRepo := jobrepo.NewStepRunRepo(db)
	executionEngine := &conductor.ExecutionEngine{Client: conductorClient}
	jobSvc := jobsvc.NewService(jobRepo, stepRunRepo, txRunner, wfRepo, &executionTemplateLookupAdapter{svc: tplSvc}, materialsClient, cfg.GetJob(), compilationRepo, executionEngine)
	jobSvc.ConfigureHistoryAssets(materialsClient)
	jobSvc.ConfigureCandidateMedia(materialsClient)
	controlClient := client.NewControlClient(os.Getenv("MEDIA_CONTROL_URL"), os.Getenv("CONTROL_INTERNAL_TOKEN"))
	resourceAuthURL := os.Getenv("MEDIA_AUTH_URL")
	if resourceAuthURL == "" {
		resourceAuthURL = "http://127.0.0.1:8001"
	}
	resourceAuthority := client.NewResourceAuthorityClient(resourceAuthURL, os.Getenv("MEDIA_AUTH_INTERNAL_TOKEN"), controlClient)
	tplSvc.ConfigureResourceAuthority(resourceAuthority.TemplateAuthority())
	wfSvc.ConfigureResourceAuthority(resourceAuthority)
	jobSvc.ConfigureWorkflowAccess(wfSvc.AuthorizeExecution)
	jobSvc.ConfigureHistoryWorkspaces(controlClient)
	jobSvc.ConfigureListWorkspaces(controlClient.ListWorkflowWorkspaceIDs)
	jobSvc.ConfigureResume(jobRepo, controlClient, conductorClient, os.Getenv("MEDIA_STORAGE_PUBLIC_ENDPOINT"))
	jobSvc.ConfigureReviews(jobRepo)
	jobSvc.ConfigureStarts(jobRepo)
	jobSvc.ConfigureSubmissions(jobRepo)
	jobhandler.Svc = jobSvc
	historypushhandler.Svc = historypushsvc.New(historypushrepo.New(db), jobRepo, controlClient, materialsClient)
	resumeCtx, resumeCancel := context.WithCancel(context.Background())
	defer resumeCancel()
	publicationFlow := &publication.Delivery{Intents: compilationRepo, Workflows: wfRepo, Engine: conductorClient, Tx: txRunner}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			deliveryCtx, cancel := context.WithTimeout(resumeCtx, 20*time.Second)
			if err := publicationFlow.DispatchOne(deliveryCtx); err != nil && resumeCtx.Err() == nil {
				log.Print("publication delivery deferred; inspect durable publication status")
			}
			cancel()
			select {
			case <-resumeCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	startFlow := &startdelivery.Flow{Starts: jobRepo, Jobs: jobRepo, Engine: executionEngine}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			deliveryCtx, cancel := context.WithTimeout(resumeCtx, 20*time.Second)
			if err := startFlow.DispatchOne(deliveryCtx); err != nil && resumeCtx.Err() == nil {
				log.Print("execution start delivery deferred; inspect durable start status")
			}
			cancel()
			select {
			case <-resumeCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	reviewFlow := &reviewdelivery.Flow{Reviews: jobRepo, Jobs: jobRepo, Engine: executionEngine}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			deliveryCtx, cancel := context.WithTimeout(resumeCtx, 20*time.Second)
			if err := reviewFlow.DispatchOne(deliveryCtx); err != nil && resumeCtx.Err() == nil {
				log.Print("review delivery deferred; inspect durable review status")
			}
			cancel()
			select {
			case <-resumeCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			if err := jobSvc.DispatchPendingResume(resumeCtx); err != nil && resumeCtx.Err() == nil {
				log.Printf("historical resume dispatch: %v", err)
			}
			select {
			case <-resumeCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	dispatchRepo := jobrepo.NewDispatchRepo(db)
	if pending, err := dispatchRepo.ListPendingResults(context.Background(), 1); err != nil || len(pending) != 0 {
		log.Fatal("legacy worker results remain unconfirmed; resolve before switching to direct delivery")
	}
	workerFlow := &workergateway.Flow{
		ArtifactUploads: materialsClient,
		StorageOrigin:   os.Getenv("MEDIA_STORAGE_PUBLIC_ENDPOINT"),
		Dispatches:      dispatchRepo, Jobs: jobRepo, StepRuns: stepRunRepo,
		Capabilities: capabilityRepo, Engine: executionEngine, Executions: executionEngine, ImportedAssets: materialsClient,
	}
	workerhandler.Flow = workerFlow

	// auth 中间件（OAuth client + cookie JWT + Bearer service token）
	mediaAuthURL := "http://127.0.0.1:8001"
	if c, ok := cfg.GetClient()["media_auth"]; ok && c.GetBaseDomain() != "" {
		mediaAuthURL = c.GetBaseDomain()
	}
	wfAddr := cfg.GetServer().GetAddress()
	if strings.HasPrefix(wfAddr, ":") {
		wfAddr = "127.0.0.1" + wfAddr
	}
	serviceURL := os.Getenv("MW_PUBLIC_URL")
	if serviceURL == "" {
		serviceURL = "http://" + wfAddr
	}
	authCfg := &auth.Config{
		JWTSecret:              cfg.GetAuth().GetJwtSecret(),
		CookieName:             "mw_token",
		JWTExpiresSec:          int(cfg.GetAuth().GetJwtExpiresInSeconds()),
		MediaAuthURL:           mediaAuthURL,
		ServiceURL:             strings.TrimRight(serviceURL, "/"),
		OAuthClientID:          cfg.GetApp().GetName(),
		OAuthClientSecret:      cfg.GetAuth().GetOauthClientSecret(),
		KratosPublicURL:        os.Getenv("ORY_KRATOS_PUBLIC_URL"),
		MediaAuthInternalToken: os.Getenv("MEDIA_AUTH_INTERNAL_TOKEN"),
		DevUserID:              os.Getenv("MW_DEV_USER_ID"),
		// skip 运维端点 + internal RPC（服务间调用不带 user session，靠网络隔离）
		SkipPaths: map[string]bool{
			"/health": true, "/metrics": true, "/ping": true, "/openapi.yaml": true,
			"/api/auth/login": true, "/api/auth/callback": true, "/api/auth/logout": true,
		},
		SkipPrefixes: []string{"/swagger/", "/debug/pprof/", "/api/internal/"},
	}
	if jwksURL := os.Getenv("INTERNAL_JWKS_URL"); jwksURL != "" {
		authCfg.InternalJWTVerifier = internaljwt.NewVerifier(jwksURL, os.Getenv("INTERNAL_JWT_ISSUER"), "internal-api", nil)
		authCfg.WorkerJWTVerifier = internaljwt.NewVerifier(jwksURL, os.Getenv("INTERNAL_JWT_ISSUER"), "media-workflow", nil)
	}
	if jwksURL := os.Getenv("AGENT_JWKS_URL"); jwksURL != "" {
		if os.Getenv("AGENT_JWT_ISSUER") == "" {
			log.Fatal("Agent JWT issuer is required")
		}
		authCfg.AgentJWTVerifier = internaljwt.NewVerifier(jwksURL, os.Getenv("AGENT_JWT_ISSUER"), "media-workflow-agent", nil)
	}
	if delegation := cfg.GetMcpDelegation(); delegation.GetEnabled() {
		if delegation.GetJwksUrl() == "" || delegation.GetIssuer() == "" || len(delegation.GetServiceToken()) < 32 {
			log.Fatal("invalid MCP delegation configuration")
		}
		admission := mcpworkflow.MCPAdmission{Verifier: internaljwt.NewVerifier(delegation.GetJwksUrl(), delegation.GetIssuer(), mcpdelegation.Workflow, nil), ServiceToken: delegation.GetServiceToken(), Facts: resourceaccess.New(mediaAuthURL, os.Getenv("MEDIA_AUTH_INTERNAL_TOKEN"), nil), Workspaces: resourceaccess.WorkspaceClient{BaseURL: os.Getenv("MEDIA_CONTROL_URL"), Credential: os.Getenv("CONTROL_INTERNAL_TOKEN")}}
		authCfg.MCPAuthenticate = admission.Authenticate
	}
	authMw := auth.New(authCfg)

	h := suite.NewServer()
	// auth 路由（不经 auth 中间件）
	h.GET("/api/auth/login", authMw.LoginHandler)
	h.GET("/api/auth/callback", authMw.CallbackHandler)
	h.POST("/api/auth/logout", authMw.LogoutHandler)
	// 受保护路由挂 auth 中间件
	h.Use(authMw.Handler())
	h.GET("/api/auth/me", authMw.MeHandler)

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

	log.Printf("media-workflow: listening on %s", cfg.GetServer().GetAddress())
	h.Spin()
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

// 编译期断言：adapter 实现 workflow.TemplateLookup 端口。
var _ wfdomain.TemplateLookup = (*templateLookupAdapter)(nil)
var _ shared.AssetKindLookup = (*client.MaterialsClient)(nil)

// job 装配（P3）

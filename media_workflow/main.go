// main.go 是 media_workflow 程序入口。
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_workflow/biz/dal"
	jobrepo "media_agent/media_workflow/biz/dal/repo/job"
	tplrepo "media_agent/media_workflow/biz/dal/repo/template"
	wfrepo "media_agent/media_workflow/biz/dal/repo/workflow"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	jobhandler "media_agent/media_workflow/biz/handler/job"
	tplhandler "media_agent/media_workflow/biz/handler/template"
	wfhandler "media_agent/media_workflow/biz/handler/workflow"
	jobsvc "media_agent/media_workflow/biz/service/job"
	tplsvc "media_agent/media_workflow/biz/service/template"
	wfsvc "media_agent/media_workflow/biz/service/workflow"
	"media_agent/media_workflow/biz/shared"
	"media_agent/media_workflow/biz/shared/auth"
	"media_agent/media_workflow/biz/shared/client"
)

// templateLookupAdapter 把 template.Service 适配为 workflow.TemplateLookup 端口。
type templateLookupAdapter struct{ svc *tplsvc.Service }

func (a *templateLookupAdapter) GetVersion(ctx context.Context, id string, version int) (*wfdomain.TemplateSnapshot, error) {
	tv, err := a.svc.GetVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	slots := make([]wfdomain.SlotSnapshot, len(tv.Inputs))
	for i, in := range tv.Inputs {
		slots[i] = wfdomain.SlotSnapshot{Name: in.Name, ResourceType: in.ResourceType, Required: in.Required}
	}
	// status 需从当前 template 取（version 快照无 status；查当前）
	t, err := a.svc.Get(ctx, id, "") // internal lookup, no caller check
	if err != nil {
		return nil, err
	}
	return &wfdomain.TemplateSnapshot{
		ID: tv.TemplateID, Scope: t.Scope, Status: string(t.Status),
		OutputAssetKind: tv.OutputAssetKind, PromptBody: tv.PromptBody, Platform: tv.Platform,
		GenWay: tv.GenWay, GenType: tv.GenType,
		NeedsConfirm: tv.NeedsConfirm, CandidateCount: tv.CandidateCount, Inputs: slots,
	}, nil
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
	tplRepo := tplrepo.NewRepo(db)
	tplGrantRepo := tplrepo.NewGrantRepo(db)
	tplSvc := tplsvc.NewService(tplRepo, tplGrantRepo, txRunner, materialsClient)
	tplhandler.Svc = tplSvc

	// workflow 装配（TemplateLookup 由 templateSvc 适配）
	tplLookup := &templateLookupAdapter{svc: tplSvc}
	wfRepo := wfrepo.NewRepo(db)
	wfGrantRepo := wfrepo.NewGrantRepo(db)
	wfSvc := wfsvc.NewService(wfRepo, wfGrantRepo, txRunner, tplLookup)
	wfhandler.Svc = wfSvc

	// job 装配（P3）
	jobRepo := jobrepo.NewJobRepo(db)
	stepRunRepo := jobrepo.NewStepRunRepo(db)
	jobSvc := jobsvc.NewService(jobRepo, stepRunRepo, txRunner, wfRepo, tplLookup, materialsClient, cfg.GetJob())
	jobhandler.Svc = jobSvc

	// claim 超时扫描（后台 goroutine）
	scannerCtx, scannerCancel := context.WithCancel(context.Background())
	defer scannerCancel()
	go jobSvc.StartClaimTimeoutScanner(scannerCtx)

	// auth 中间件（OAuth client + cookie JWT + Bearer service token）
	mediaAuthURL := "http://127.0.0.1:8001"
	if c, ok := cfg.GetClient()["media_auth"]; ok && c.GetBaseDomain() != "" {
		mediaAuthURL = c.GetBaseDomain()
	}
	wfAddr := cfg.GetServer().GetAddress()
	if strings.HasPrefix(wfAddr, ":") {
		wfAddr = "127.0.0.1" + wfAddr
	}
	authCfg := &auth.Config{
		JWTSecret:         cfg.GetAuth().GetJwtSecret(),
		CookieName:        "mw_token",
		JWTExpiresSec:     int(cfg.GetAuth().GetJwtExpiresInSeconds()),
		MediaAuthURL:      mediaAuthURL,
		ServiceURL:        "http://" + wfAddr,
		OAuthClientID:     cfg.GetApp().GetName(),
		OAuthClientSecret: cfg.GetAuth().GetOauthClientSecret(),
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

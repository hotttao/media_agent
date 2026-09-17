package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_materials/biz/dal"
	assetkindrepo "media_agent/media_materials/biz/dal/repo/asset_kind"
	genassetrepo "media_agent/media_materials/biz/dal/repo/generated_asset"
	materialrepo "media_agent/media_materials/biz/dal/repo/material"
	productrepo "media_agent/media_materials/biz/dal/repo/product"
	viprepo "media_agent/media_materials/biz/dal/repo/virtual_ip"
	assetkindhandler "media_agent/media_materials/biz/handler/asset_kind"
	genassethandler "media_agent/media_materials/biz/handler/generated_asset"
	materialhandler "media_agent/media_materials/biz/handler/material"
	producthandler "media_agent/media_materials/biz/handler/product"
	reachhandler "media_agent/media_materials/biz/handler/reachability"
	viphandler "media_agent/media_materials/biz/handler/virtual_ip"
	assetkindsvc "media_agent/media_materials/biz/service/asset_kind"
	genassetsvc "media_agent/media_materials/biz/service/generated_asset"
	materialsvc "media_agent/media_materials/biz/service/material"
	productsvc "media_agent/media_materials/biz/service/product"
	vipsvc "media_agent/media_materials/biz/service/virtual_ip"
	"media_agent/media_materials/biz/shared"
	"media_agent/media_materials/biz/shared/auth"
	wflow "media_agent/media_materials/biz/workflow/reachability"
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

	// service 装配（repo → service → handler 注入）
	assetKindRepo := assetkindrepo.NewRepo(db)
	assetKindSvc := assetkindsvc.NewService(assetKindRepo)
	assetkindhandler.Svc = assetKindSvc

	productRepo := productrepo.NewRepo(db)
	productSvc := productsvc.NewService(productRepo)
	producthandler.Svc = productSvc

	vipRepo := viprepo.NewRepo(db)
	vipGrantRepo := viprepo.NewGrantRepo(db)
	vipSvc := vipsvc.NewService(vipRepo, vipGrantRepo)
	viphandler.Svc = vipSvc

	ownerScopeLookup := shared.NewOwnerScopeLookup(vipSvc.GetOwnerScope, productSvc.GetOwnerScope)
	materialRepo := materialrepo.NewRepo(db)
	materialBindingRepo := materialrepo.NewBindingRepo(db)
	materialSvc := materialsvc.NewService(materialRepo, materialBindingRepo, assetKindSvc, ownerScopeLookup)
	materialhandler.Svc = materialSvc
	// owner 软删时失效其 material_bindings（跨 domain 写，接口注入，见 shared.BindingEnder）。
	vipSvc.SetBindingEnder(materialSvc)
	productSvc.SetBindingEnder(materialSvc)

	genAssetRepo := genassetrepo.NewRepo(db)
	genAssetSvc := genassetsvc.NewService(genAssetRepo)
	genassethandler.Svc = genAssetSvc

	// reachability workflow：编排 material/generated_asset/vip/product 只读 + policy 决策。
	// 供 caller（BFF）组装 JobInput bundle 时调用（ADR 0006）。
	reachFlow := wflow.NewFlow(materialSvc, genAssetSvc, vipSvc, productSvc, assetKindSvc)
	reachhandler.FlowSvc = reachFlow

	// auth 中间件（OAuth client + cookie JWT + Bearer service token）
	mediaAuthURL := "http://127.0.0.1:8001"
	if c, ok := cfg.GetClient()["media_auth"]; ok && c.GetBaseDomain() != "" {
		mediaAuthURL = c.GetBaseDomain()
	}
	addr := cfg.GetServer().GetAddress()
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	serviceURL := "http://" + addr
	if publicURL := os.Getenv("MW_PUBLIC_URL"); publicURL != "" {
		serviceURL = strings.TrimRight(publicURL, "/")
	}
	authCfg := &auth.Config{
		JWTSecret:         cfg.GetAuth().GetJwtSecret(),
		CookieName:        "mw_token",
		JWTExpiresSec:     int(cfg.GetAuth().GetJwtExpiresInSeconds()),
		MediaAuthURL:      mediaAuthURL,
		ServiceURL:        serviceURL,
		OAuthClientID:     cfg.GetApp().GetName(),
		OAuthClientSecret: cfg.GetAuth().GetOauthClientSecret(),
		DevUserID:         os.Getenv("MW_DEV_USER_ID"),
		// skip 运维端点 + internal RPC（服务间调用不带 user session，靠网络隔离）
		SkipPaths: map[string]bool{
			"/health": true, "/metrics": true, "/ping": true, "/openapi.yaml": true,
			"/api/auth/login": true, "/api/auth/callback": true, "/api/auth/logout": true,
		},
		SkipPrefixes: []string{"/swagger/", "/debug/pprof/", "/api/internal/"},
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

	log.Printf("media-materials: listening on %s", cfg.GetServer().GetAddress())
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

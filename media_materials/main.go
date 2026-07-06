package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

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
	viphandler "media_agent/media_materials/biz/handler/virtual_ip"
	assetkindsvc "media_agent/media_materials/biz/service/asset_kind"
	genassetsvc "media_agent/media_materials/biz/service/generated_asset"
	materialsvc "media_agent/media_materials/biz/service/material"
	productsvc "media_agent/media_materials/biz/service/product"
	vipsvc "media_agent/media_materials/biz/service/virtual_ip"
	"media_agent/media_materials/biz/shared/auth"
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

	materialRepo := materialrepo.NewRepo(db)
	materialBindingRepo := materialrepo.NewBindingRepo(db)
	materialSvc := materialsvc.NewService(materialRepo, materialBindingRepo, assetKindSvc)
	materialhandler.Svc = materialSvc

	productRepo := productrepo.NewRepo(db)
	productSvc := productsvc.NewService(productRepo)
	producthandler.Svc = productSvc

	genAssetRepo := genassetrepo.NewRepo(db)
	genAssetSvc := genassetsvc.NewService(genAssetRepo)
	genassethandler.Svc = genAssetSvc

	vipRepo := viprepo.NewRepo(db)
	vipGrantRepo := viprepo.NewGrantRepo(db)
	vipSvc := vipsvc.NewService(vipRepo, vipGrantRepo)
	viphandler.Svc = vipSvc

	// auth 中间件（OAuth client + cookie JWT + Bearer service token）
	authCfg := &auth.Config{
		JWTSecret:     cfg.GetAuth().GetJwtSecret(),
		CookieName:    "mw_token",
		JWTExpiresSec: 7 * 24 * 3600,
		MediaAuthURL:  "http://127.0.0.1:8001",
		ServiceURL:    "http://127.0.0.1:8003",
		OAuthClientID: "media_materials",
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

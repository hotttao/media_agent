package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_materials/biz/dal"
	assetkindrepo "media_agent/media_materials/biz/dal/repo/asset_kind"
	materialrepo "media_agent/media_materials/biz/dal/repo/material"
	productrepo "media_agent/media_materials/biz/dal/repo/product"
	viprepo "media_agent/media_materials/biz/dal/repo/virtual_ip"
	libraryobjecthandler "media_agent/media_materials/biz/handler/library_object"
	materialhandler "media_agent/media_materials/biz/handler/material"
	producthandler "media_agent/media_materials/biz/handler/product"
	viphandler "media_agent/media_materials/biz/handler/virtual_ip"
	assetkindsvc "media_agent/media_materials/biz/service/asset_kind"
	materialsvc "media_agent/media_materials/biz/service/material"
	productsvc "media_agent/media_materials/biz/service/product"
	vipsvc "media_agent/media_materials/biz/service/virtual_ip"
	"media_agent/media_materials/biz/shared/auth"
	"media_agent/media_materials/biz/shared/objectstorage"
	uploadworkflow "media_agent/media_materials/biz/workflow/library_object_upload"
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

	productRepo := productrepo.NewRepo(db)
	productSvc := productsvc.NewService(productRepo)
	producthandler.Svc = productSvc

	vipRepo := viprepo.NewRepo(db)
	vipGrantRepo := viprepo.NewGrantRepo(db)
	vipSvc := vipsvc.NewService(vipRepo, vipGrantRepo)
	viphandler.Svc = vipSvc

	materialRepo := materialrepo.NewRepo(db)
	materialSvc := materialsvc.NewService(materialRepo, nil, assetKindSvc, nil)
	materialhandler.Svc = materialSvc

	storageCfg := cfg.GetObjectStorage()
	if storageCfg == nil || !storageCfg.GetEnabled() {
		log.Fatal("object_storage is required")
	}
	storageClient, err := objectstorage.New(context.Background(), objectstorage.Config{
		Provider: storageCfg.GetProvider(), Endpoint: storageCfg.GetEndpoint(), PublicEndpoint: storageCfg.GetPublicEndpoint(),
		Region: storageCfg.GetRegion(), Bucket: storageCfg.GetBucket(), AccessKey: storageCfg.GetAccessKey(), SecretKey: storageCfg.GetSecretKey(),
		ForcePathStyle: storageCfg.GetForcePathStyle(), PresignTTL: time.Duration(storageCfg.GetPresignTtlSeconds()) * time.Second,
	})
	if err != nil {
		log.Fatalf("init object storage: %v", err)
	}
	libraryobjecthandler.Flow = uploadworkflow.New(storageClient)
	libraryobjecthandler.Resolver = storageClient

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
		JWTSecret:              cfg.GetAuth().GetJwtSecret(),
		CookieName:             "mw_token",
		JWTExpiresSec:          int(cfg.GetAuth().GetJwtExpiresInSeconds()),
		MediaAuthURL:           mediaAuthURL,
		ServiceURL:             serviceURL,
		OAuthClientID:          cfg.GetApp().GetName(),
		OAuthClientSecret:      cfg.GetAuth().GetOauthClientSecret(),
		KratosPublicURL:        os.Getenv("ORY_KRATOS_PUBLIC_URL"),
		MediaAuthInternalToken: os.Getenv("MEDIA_AUTH_INTERNAL_TOKEN"),
		DevUserID:              os.Getenv("MW_DEV_USER_ID"),
		DevOrganizationID:      os.Getenv("MW_DEV_ORGANIZATION_ID"),
		// skip 运维端点 + internal RPC（服务间调用不带 user session，靠网络隔离）
		SkipPaths: map[string]bool{
			"/health": true, "/metrics": true, "/ping": true, "/openapi.yaml": true,
			"/api/auth/login": true, "/api/auth/callback": true, "/api/auth/logout": true,
		},
		SkipPrefixes: []string{"/swagger/", "/debug/pprof/", "/api/internal/"},
	}
	if jwksURL := os.Getenv("INTERNAL_JWKS_URL"); jwksURL != "" {
		authCfg.InternalJWTVerifier = internaljwt.NewVerifier(jwksURL, os.Getenv("INTERNAL_JWT_ISSUER"), "internal-api", nil)
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

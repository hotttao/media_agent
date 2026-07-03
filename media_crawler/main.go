package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_crawler/biz/dal"
	"media_agent/media_crawler/biz/dal/query"
	"media_agent/media_crawler/biz/dal/query_read"
	crawldb "media_agent/media_crawler/biz/dal/repo/crawl_job"
	xhsdb "media_agent/media_crawler/biz/dal/repo/xhs_data"
	crawlerhandler "media_agent/media_crawler/biz/handler/crawler"
	mcpserver "media_agent/media_crawler/biz/handler/mcp"
	xhshandler "media_agent/media_crawler/biz/handler/xhs"
	crawljob "media_agent/media_crawler/biz/service/crawl_job"
	xhsdata "media_agent/media_crawler/biz/service/xhs_data"
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
	_ = db

	// 爬虫 MySQL（只读，跨库查内容数据）
	crawlDataDB, err := dal.NewCrawlDataDB(cfg.GetCrawlData())
	if err != nil {
		log.Fatalf("init crawl data db: %v", err)
	}
	query.SetDefault(db)
	query_read.SetDefault(crawlDataDB)

	// service 装配（repo → service → handler 注入）
	jobRepo := crawldb.NewJobRepo(db)
	txRunner := dal.NewTxRunner(db)
	crawlSvc := crawljob.NewService(jobRepo, txRunner, cfg.GetCrawler())
	xhsRepo := xhsdb.NewXhsRepo(crawlDataDB)
	xhsSvc := xhsdata.NewService(xhsRepo)

	crawlerhandler.Svc = crawlSvc
	xhshandler.Svc = xhsSvc

	h := suite.NewServer()
	register(h)
	suite.RegisterRoutes(h)
	mcpserver.Register(h, crawlSvc, xhsSvc)

	if err := suite.RegisterService(); err != nil {
		log.Fatalf("register service: %v", err)
	}
	defer func() {
		if err := suite.DeregisterService(); err != nil {
			log.Printf("deregister service: %v", err)
		}
	}()

	log.Printf("media-crawler: listening on %s", cfg.GetServer().GetAddress())
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

// main.go 是 media_gen 程序入口。
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_gen/biz/dal"
	publishhandler "media_agent/media_gen/biz/handler/publish"
	workerhandler "media_agent/media_gen/biz/handler/worker"
	gensvc "media_agent/media_gen/biz/service/generation"
	publishsvc "media_agent/media_gen/biz/service/publish"
	workersvc "media_agent/media_gen/biz/service/worker"
	"media_agent/media_gen/biz/shared/client"
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

	// media_gen 本期不持久化（generation / publish 均无表），但保留 DB 装配骨架
	// 供后续真实实现（如 publish 异步轮询状态表、worker 注册表持久化）使用。
	dbCfg := cfg.GetDatabase()
	db, err := dal.NewDB(dbCfg)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	if err := dal.Migrate("migrations", dbCfg); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	_ = db

	// 下游 client 装配：media_workflow.job_service（ClaimStepRun / CompleteStepRun / FailStepRun）。
	// 配置缺失时 log.Fatal - 无下游则 worker loop 无法运行。
	client.Init(cfg)

	// generation service：实现 worker.StepExecutor 端口；repo = workflow client。
	workflowRepo := client.NewWorkflowClient(client.MediaWorkflowJobClient)
	genSvc := gensvc.NewService(workflowRepo)

	// worker service：进程内 worker 注册表 + goroutine 生命周期。
	// loop 本期 stub（不实际跑 Claim 循环），executor 注入就绪供真实实现接入。
	workerSvc := workersvc.NewService(genSvc)
	workerhandler.Svc = workerSvc

	// publish service：mock 发布，无下游依赖。
	publishSvc := publishsvc.NewService()
	publishhandler.Svc = publishSvc

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

	log.Printf("media-gen: listening on %s", cfg.GetServer().GetAddress())
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

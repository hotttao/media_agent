// main.go 是 Hertz 程序入口。
//
// 所有服务端治理能力收口在 ServerSuite 上，main.go 只按顺序调用其方法
// （详见 hertz_infra/serverhertz/suite.go 包注释）：
//  1. config.NewLoader 加载配置；
//  2. dal.NewDB + dal.Migrate：数据库初始化 + 启动期迁移（database.migrate_on_start 门控，幂等）；
//  3. serverhertz.NewServerSuite 构造 ServerSuite；
//  4. loader.Attach(KVSource) + loader.Watch 启动配置热更新；
//  5. suite.InitObservability 进程级可观测性初始化（logger + tracing + metrics）；
//  6. client.Init 初始化下游 client 装配层；
//  7. suite.NewServer 创建 server + 挂全局 middleware；
//  8. InitDI(db) 注入 handler 端 service / workflow 实例（必须在 register 之前）；
//  9. register(h) 注册生成的业务路由；
//  10. suite.RegisterRoutes 注册运维端点（health / pprof / metrics / swagger）；
//  11. suite.RegisterService 注册服务，defer 反注册；
//  12. h.Spin。
//
// main.go 只做启动编排，不直接拼 options / middleware / 注册中心。
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/shared/client"
)

func main() {
	// 1. 创建 config Loader（一次调用闭环：.env + APP_ENV → 读文件 + 首次加载）。
	//    Loader 内部自动创建 file source，但不启动 watch——watch 由 Watch() 显式启动。
	loader, err := config.NewLoader()
	if err != nil {
		log.Fatalf("init config loader: %v", err)
	}
	defer loader.Close()

	// 2. 数据库初始化 + 启动期迁移（database.enabled=false 时 NewDB 返回 nil；migrate_on_start 门控）。
	//    迁移幂等：已最新 no-op，落后执行到最新。必须在 NewServer / InitDI 之前，确保 schema 就绪。
	dbCfg := loader.Current().GetDatabase()
	db, err := dal.NewDB(dbCfg)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	if err := dal.Migrate("migrations", dbCfg); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	// 3. 构造 ServerSuite（收 loader 不收 cfg；可热更新能力组件在 suite 内部自行 loader.Subscribe）。
	suite := serverhertz.NewServerSuite(loader)

	// 3. Attach Consul KV source（consul_kv.enabled=false 时 KVSource() 返回 nil，Attach 无操作）。
	if err := loader.Attach(suite.KVSource()); err != nil {
		log.Fatalf("attach kv source: %v", err)
	}

	// 4. 启动所有 source 的 watch（文件 fsnotify + Consul KV blocking-query）。
	if err := loader.Watch(); err != nil {
		log.Fatalf("start config watch: %v", err)
	}

	// 5. 进程级可观测性初始化（logger + tracing + metrics），必须在 NewServer 之前完成。
	//    cfg.Log.Dir 为空时 fallback 到 <repo>/media_logs/<app.name>（向上找 go.work）；
	//    proto message 原地修改，loader.Current() 后续读到的就是新值。
	cfg := loader.Current()
	if cfg.GetLog().GetDir() == "" {
		cfg.GetLog().Dir = resolveDefaultLogDir(cfg.GetApp().GetName())
	}
	shutdownObs, err := suite.InitObservability()
	if err != nil {
		log.Fatalf("init observability: %v", err)
	}
	defer shutdownObs(context.Background())

	// 6. 初始化下游 client 装配层（当前模板无下游，baseSuite 仅做进程级装配）。
	if _, err := client.Init(loader); err != nil {
		log.Fatalf("init clients: %v", err)
	}

	// 7. 创建 Hertz server + 挂全局 middleware。
	h := suite.NewServer()

	// 8. 依赖注入：把 service / workflow 装配到 handler 包级变量。
	//    必须在 register(h) 之前完成，确保首个请求到达时依赖非 nil。
	InitDI(db)

	// 9. 注册生成的业务路由（hz update 由 router_gen.go + biz/router/register.go 提供）。
	register(h)

	// 10. 注册运维端点（health / pprof / metrics route），按 enabled 门控。
	suite.RegisterRoutes(h)

	// 11. 服务注册（consul.enabled=true 时生效），defer 反注册。
	if err := suite.RegisterService(); err != nil {
		log.Fatalf("register service: %v", err)
	}
	defer func() {
		if err := suite.DeregisterService(); err != nil {
			log.Printf("deregister service: %v", err)
		}
	}()

	// 12. 启动。
	h.Spin()
}

// resolveDefaultLogDir 在 cfg.GetLog().Dir 为空时使用：
// 向上找 go.work 确定仓库根，返回 <root>/media_logs/<appName>；
// 找不到仓库根时返回空（globalhertz.InitLogger 见空仅写 stdout）。
func resolveDefaultLogDir(appName string) string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
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

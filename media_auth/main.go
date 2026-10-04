// main.go 是 media_auth 微服务入口。
//
// 所有服务端治理能力收口在 ServerSuite 上，main.go 只按顺序调用其方法
// （详见 hertz_infra/serverhertz/suite.go 包注释）：
//  1. config.NewLoader 加载配置；
//  2. dal.NewDB + dal.Migrate：数据库初始化 + 启动期迁移（database.migrate_on_start 门控，幂等）；
//  3. serverhertz.NewServerSuite 构造 ServerSuite；
//  4. loader.Attach(KVSource) + loader.Watch 启动配置热更新；
//  5. suite.InitObservability 进程级可观测性初始化（logger + tracing + metrics）；
//  6. suite.NewServer 创建 server + 挂全局 middleware；
//  7. InitDI() 注入 handler 端 service / workflow 实例（必须在 register 之前）；
//  8. register(h) 注册生成的业务路由；
//  9. suite.RegisterRoutes 注册运维端点（health / pprof / metrics / swagger）；
//  10. suite.RegisterService 注册服务，defer 反注册；
//  11. h.Spin。
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
	"media_agent/media_auth/biz/dal"
)

func main() {
	// 1. 创建 config Loader（一次调用闭环：.env + APP_ENV → 读文件 + 首次加载）。
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

	// 4. Attach Consul KV source（consul_kv.enabled=false 时 KVSource() 返回 nil，Attach 无操作）。
	if err := loader.Attach(suite.KVSource()); err != nil {
		log.Fatalf("attach kv source: %v", err)
	}

	// 5. 启动所有 source 的 watch（文件 fsnotify + Consul KV blocking-query）。
	if err := loader.Watch(); err != nil {
		log.Fatalf("start config watch: %v", err)
	}

	// 6. 进程级可观测性初始化（logger + tracing + metrics），必须在 NewServer 之前完成。
	cfg := loader.Current()
	if cfg.GetLog().GetDir() == "" {
		cfg.GetLog().Dir = resolveDefaultLogDir(cfg.GetApp().GetName())
	}
	shutdownObs, err := suite.InitObservability()
	if err != nil {
		log.Fatalf("init observability: %v", err)
	}
	defer shutdownObs(context.Background())

	// 7. 创建 Hertz server + 挂全局 middleware。
	h := suite.NewServer()

	// 8. 依赖注入：把 service / workflow 装配到 handler 包级变量。
	//    必须在 register(h) 之前完成，确保首个请求到达时依赖非 nil。
	platform, err := newPlatformRuntime(db, cfg.GetPlatformToken())
	if err != nil {
		log.Fatal("platform token runtime initialization failed")
	}
	defer platform.close()
	InitDI(db, cfg, platform)

	// 8.1 挂 session 鉴权中间件（cookie JWT 验签 + 注入 ctx + 滑动续期 + skip list）。
	//     在业务路由注册前挂载，所有受保护路由经此鉴权。
	if SessionMiddleware != nil {
		h.Use(SessionMiddleware)
	}

	// 9. 注册生成的业务路由（hz update 由 router_gen.go + biz/router/register.go 提供）。
	register(h)

	// 10. 注册运维端点（health / pprof / metrics / swagger），按 enabled 门控。
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
	if platform != nil {
		platform.start()
		go func() {
			if err := platform.server.Run(); err != nil && !platform.stopping.Load() {
				log.Fatal("platform token internal listener failed")
			}
		}()
	}
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

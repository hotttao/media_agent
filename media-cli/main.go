// media-cli 是 media_agent 的命令行工具，基于 spf13/cobra。
//
// 启动流程（与 media-example/main.go 一致，复用 hertz_infra/config）：
//  1. chdir 到 media-cli/（让 config.NewLoader 的相对路径 conf/<env>.yaml 能解析到）；
//  2. config.NewLoader() 加载 .env + conf/<APP_ENV>.yaml，得到 *config.Config；
//  3. main 调 globalhertz.InitLogger + InitOTel 初始化进程级可观测性（logger + OTel provider）；
//  4. main 调各 subpackage 的 Init(cfg)：
//     - hello.Init(cfg)：构造共享 HelloService typed client（后续 method1/method2/method3 复用）；
//     - client.Init(cfg)：注入 App/Otel 段供 e2e 测试 clienthertz 治理装配用；
//  5. cmd.Execute() 启动 cobra 命令树。
//
// 「config init 按职责下沉」约定（与用户对齐后）：
//   - main.go 是 cfg 的消费者与分发者；
//   - 每个需要 cfg 的子系统在自己的目录下导出 Init(cfg) 函数；
//   - subpackage 不持 cfg 引用，只通过 Init 捕获自己需要的字段到包级 var；
//   - observability 包（infra 包）只提供 WithSpan 等工具，不参与 cfg 初始化。
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/globalhertz"
	"media_agent/media_cli/cmd"

	// Blank imports 触发各 subpackage 的 init() 注册子命令到 rootCmd。
	// Init(cfg) 由 main 显式调用（不依赖 init()）。
	_ "media_agent/media_cli/cmd/client"
	_ "media_agent/media_cli/cmd/infra"
	_ "media_agent/media_cli/cmd/media_agent_gateway"     // Agent Gateway 接口 e2e
	_ "media_agent/media_cli/cmd/media_agents"            // Agent Runtime 内部 API e2e
	_ "media_agent/media_cli/cmd/media_auth"              // media_auth 接口 e2e 一级子命令
	_ "media_agent/media_cli/cmd/media_auth/auth"         // auth 子命令挂到 MediaAuthCmd
	_ "media_agent/media_cli/cmd/media_auth/oauth_client" // oauth-client 子命令挂到 MediaAuthCmd
	_ "media_agent/media_cli/cmd/media_auth/team"         // team 子命令挂到 MediaAuthCmd
	_ "media_agent/media_cli/cmd/media_auth/user"         // user 子命令挂到 MediaAuthCmd
	_ "media_agent/media_cli/cmd/media_control"           // Media Control 接口 e2e
	_ "media_agent/media_cli/cmd/media_example"
	// hello 是 media_example 的子包（service 二级）；父包 init() 把它挂到 MediaExampleCmd。
	_ "media_agent/media_cli/cmd/media_example/hello"

	// media_crawler 子命令（REST crawler/xhs + MCP tool）
	_ "media_agent/media_cli/cmd/media_crawler"
	_ "media_agent/media_cli/cmd/media_crawler/crawler"
	_ "media_agent/media_cli/cmd/media_crawler/mcp"
	_ "media_agent/media_cli/cmd/media_crawler/xhs"

	// media_workflow caller（编排 bundle + SubmitJob）
	_ "media_agent/media_cli/cmd/media_workflow"

	clisub "media_agent/media_cli/cmd/client"
	gatewaycmd "media_agent/media_cli/cmd/media_agent_gateway"
	apitokensub "media_agent/media_cli/cmd/media_auth/api_token"
	authsub "media_agent/media_cli/cmd/media_auth/auth"
	oauthclientsub "media_agent/media_cli/cmd/media_auth/oauth_client"
	teamsub "media_agent/media_cli/cmd/media_auth/team"
	usersub "media_agent/media_cli/cmd/media_auth/user"
	controlcmd "media_agent/media_cli/cmd/media_control"
	hellosub "media_agent/media_cli/cmd/media_example/hello"
	wfsub "media_agent/media_cli/cmd/media_workflow"
)

func main() {
	os.Exit(run())
}

func run() int {
	// 1. chdir 到 media-cli/，让 config.NewLoader 的相对路径 (./.env, ./conf/<env>.yaml) 能解析到。
	if err := chdirToMediaCLI(); err != nil {
		log.Fatalf("locate media-cli/: %v", err)
	}

	// 2. 加载配置（.env + APP_ENV → conf/<env>.yaml）。
	loader, err := config.NewLoader()
	if err != nil {
		log.Fatalf("init config loader: %v", err)
	}
	defer loader.Close()
	cfg := loader.Current()

	// 3. 进程级可观测性（logger + OTel provider），必须在各 Init(cfg) 之前——
	//    hello.Init 内部可能调 globalhertz 注入 trace context，client 同理。
	logCfg := cfg.GetLog()
	if logCfg.GetDir() == "" {
		// 默认 fallback：找仓库根（向上找 go.work），落 <root>/media_logs/<app.name>。
		// 这样不显式设 LOG_DIR 也能写盘（promtail 可采集）。
		logCfg.Dir = resolveDefaultLogDir(cfg.GetApp().GetName())
	}
	if err := globalhertz.InitLogger(cfg.GetApp(), logCfg); err != nil {
		log.Printf("media-cli: init logger: %v", err)
	}
	shutdownObs := globalhertz.InitOTel(cfg.GetApp(), cfg.GetOtel())
	defer func() {
		_ = shutdownObs(context.Background())
	}()

	// 4. 把 cfg 分发给需要它的子系统。
	if cfg.GetClient()["media_example"] != nil {
		hellosub.Init(cfg) // 共享 HelloService typed client
	}
	clisub.Init(cfg) // e2e 测试 clienthertz 治理参数

	// 4.1 cookie store 落盘目录 = media-cli 工作目录（chdir 后已是 media-cli/）。
	cmd.CookiesDir = "."

	// 4.2 media_auth auth 子命令的 typed client + cookie middleware 装配。
	//     client.media_auth 未配置时跳过（其他子命令仍可用）。
	if cfg.GetClient()["media_auth"] != nil {
		authsub.Init(cfg)
		if err := apitokensub.Init(cfg); err != nil {
			log.Fatalf("media_auth/api-token: %v", err)
		}
		teamsub.Init(cfg)        // team 共享同一 cookie store，login 后 team 子命令自动带 cookie
		oauthclientsub.Init(cfg) // oauth-client 共享同一 cookie store
		usersub.Init(cfg)        // user 管理共享同一 cookie store
	}
	if cfg.GetClient()["media_control"] != nil {
		controlcmd.Init(cfg)
	}
	if cfg.GetClient()["media_agent_gateway"] != nil {
		gatewaycmd.Init(cfg)
	}

	// 4.3 media_workflow caller: reachability + workflow job typed client。
	//     gate on media_materials + media_workflow 都配置（dev 缺 media_workflow 段则跳过）。
	if cfg.GetClient()["media_materials"] != nil && cfg.GetClient()["media_workflow"] != nil {
		wfsub.Init(cfg)
	}

	// 5. 启动 cobra 命令树。
	if err := cmd.Execute(); err != nil {
		log.Printf("media-cli: %v", err)
		return 1
	}
	return 0
}

// chdirToMediaCLI 把工作目录切到 media-cli/，让 config.NewLoader 的相对路径解析到。
//
// 从仓库根启动（cwd=repo root）→ 切到 ./media-cli；
// 已经在 media-cli/ 里（通过 go run . 或 cd 后再 go run）→ 不动；
// 找不到 media-cli/ → 报错。
func chdirToMediaCLI() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	if filepath.Base(wd) == "media-cli" {
		return nil
	}
	target := filepath.Join(wd, "media-cli")
	if _, err := os.Stat(target); err != nil {
		if _, err2 := os.Stat(filepath.Join(wd, "..", "media-cli")); err2 == nil {
			return os.Chdir(filepath.Join(wd, "..", "media-cli"))
		}
		return err
	}
	return os.Chdir(target)
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

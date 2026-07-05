// client.go — media_auth TeamService 的 typed client 装配 + 持久化 cookie middleware。
//
// 复用 auth 包的 cookie store 模式：cookie 落盘到 .cookies/<host>/cookies.json，
// 与 auth login 共享——login 一次后 team 子命令自动带 cookie。
// team 接口 session-based（所有端点需 session，除 invite preview/accept 需任意已登录 user）。
package team

import (
	"log"

	"media_agent/hertz_gen/media_auth/team_service"
	"media_agent/hertz_infra/clienthertz"
	config "media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
	"media_agent/media_cli/cmd/cookie"
)

var (
	teamCli team_service.Client
	store   *cookie.Store
)

// Init 由 media-cli main.go 在加载 cfg 后调用，构造 teamCli + cookie store。
// 与 auth.Init 共用同一 cookie store 落盘目录（.cookies/），跨子命令复用 cookie。
func Init(cfg *config.Config) {
	cliCfg := cfg.GetClient()["media_auth"]
	if cliCfg == nil || !cliCfg.GetEnabled() {
		log.Fatalf("media_auth/team: client.media_auth 未配置或 enabled=false")
	}
	host := cliCfg.GetBaseDomain()
	if host == "" {
		log.Fatalf("media_auth/team: client.media_auth.base_domain 为空")
	}

	base := clienthertz.NewClientSuiteFromConfig(cfg)
	appSuite, err := base.NewAppClientSuite("media_auth")
	if err != nil {
		log.Fatalf("media_auth/team: derive downstream suite: %v", err)
	}
	shared, err := appSuite.NewClient()
	if err != nil {
		log.Fatalf("media_auth/team: build shared client: %v", err)
	}

	store = cookie.New(cmd.CookiesDir)
	teamCli, err = team_service.NewTeamServiceClient(host,
		team_service.WithHertzClient(shared),
		team_service.WithHertzClientMiddleware(store.Middleware(host)),
	)
	if err != nil {
		log.Fatalf("media_auth/team: 创建 typed client 失败: %v", err)
	}
}

// Client 返回 Init 构造的共享 typed client。
func Client() team_service.Client { return teamCli }

// CookieStore 返回 cookie store（跨子命令共享 cookie）。
func CookieStore() *cookie.Store { return store }

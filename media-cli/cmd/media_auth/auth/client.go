// client.go — media_auth AuthService 的 typed client 装配 + 持久化 cookie middleware。
//
// auth 接口 session-based（login 下发 cookie，me/change-password 依赖 cookie）。
// generated typed client 不维护 cookie，故注入 cookie middleware（cmd/cookie.Store）：
//   - 请求前从 store 读该 host cookie 塞 req；
//   - 响应后读 Set-Cookie 更新 store + 落盘（跨命令复用）。
//
// 共享 client 模式：clienthertz.NewClientSuiteFromConfig → NewAppClientSuite → NewClient
// 产共享 *hclient.Client，typed client 用 WithHertzClient + WithHertzClientMiddleware(cookie) 注入。
// cookie store 按 host 隔离（.cookies/<host>/cookies.json）。
package auth

import (
	"log"

	authclient "media_agent/hertz_gen/media_auth/auth_service"
	"media_agent/hertz_infra/clienthertz"
	config "media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
	"media_agent/media_cli/cmd/cookie"
)

var (
	authCli authclient.Client
	store   *cookie.Store
)

// Init 由 media-cli main.go 在加载 cfg 后调用，构造 authCli + cookie store。
//
// cfg.Client["media_auth"] 必须 enabled=true 且有 base_domain；否则 log.Fatalf。
// cookie store 落盘到 media-cli/.cookies/（仓库根的 media-cli 目录下）。
func Init(cfg *config.Config) {
	cliCfg := cfg.GetClient()["media_auth"]
	if cliCfg == nil || !cliCfg.GetEnabled() {
		log.Fatalf("media_auth/auth: client.media_auth 未配置或 enabled=false（请检查 conf/<env>.yaml）")
	}
	host := cliCfg.GetBaseDomain()
	if host == "" {
		log.Fatalf("media_auth/auth: client.media_auth.base_domain 为空")
	}

	// 共享 client（治理烘焙）+ cookie middleware。
	base := clienthertz.NewClientSuiteFromConfig(cfg)
	appSuite, err := base.NewAppClientSuite("media_auth")
	if err != nil {
		log.Fatalf("media_auth/auth: derive downstream suite: %v", err)
	}
	shared, err := appSuite.NewClient()
	if err != nil {
		log.Fatalf("media_auth/auth: build shared client: %v", err)
	}

	// cookie store（落盘到 media-cli 目录）+ middleware 注入 typed client。
	store = cookie.New(cmd.CookiesDir)
	authCli, err = authclient.NewAuthServiceClient(host,
		authclient.WithHertzClient(shared),
		authclient.WithHertzClientMiddleware(store.Middleware(host)),
	)
	if err != nil {
		log.Fatalf("media_auth/auth: 创建 typed client 失败: %v", err)
	}
}

// Client 返回 Init 构造的共享 typed client。
func Client() authclient.Client { return authCli }

// CookieStore 返回 cookie store（logout 清凭证用）。
func CookieStore() *cookie.Store { return store }

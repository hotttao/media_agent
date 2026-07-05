// client.go — media_auth OAuthClientService typed client 装配 + 持久化 cookie middleware。
//
// 复用 auth/team 的 cookie store 模式：cookie 落盘 .cookies/<host>/，跨子命令共享。
// 需先 `media_auth auth login`（admin 账号）拿 cookie；client 管理需 admin。
package oauth_client

import (
	"log"

	oauthclientservice "media_agent/hertz_gen/media_auth/oauth_client_service"
	"media_agent/hertz_infra/clienthertz"
	config "media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
	"media_agent/media_cli/cmd/cookie"
)

var (
	clientCli oauthclientservice.Client
	store     *cookie.Store
)

// Init 由 media-cli main.go 调用，构造 typed client + cookie store。
func Init(cfg *config.Config) {
	cliCfg := cfg.GetClient()["media_auth"]
	if cliCfg == nil || !cliCfg.GetEnabled() {
		log.Fatalf("media_auth/oauth_client: client.media_auth 未配置或 enabled=false")
	}
	host := cliCfg.GetBaseDomain()
	if host == "" {
		log.Fatalf("media_auth/oauth_client: client.media_auth.base_domain 为空")
	}

	base := clienthertz.NewClientSuiteFromConfig(cfg)
	appSuite, err := base.NewAppClientSuite("media_auth")
	if err != nil {
		log.Fatalf("media_auth/oauth_client: derive downstream suite: %v", err)
	}
	shared, err := appSuite.NewClient()
	if err != nil {
		log.Fatalf("media_auth/oauth_client: build shared client: %v", err)
	}

	store = cookie.New(cmd.CookiesDir)
	clientCli, err = oauthclientservice.NewOAuthClientServiceClient(host,
		oauthclientservice.WithHertzClient(shared),
		oauthclientservice.WithHertzClientMiddleware(store.Middleware(host)),
	)
	if err != nil {
		log.Fatalf("media_auth/oauth_client: 创建 typed client 失败: %v", err)
	}
}

// Client 返回 Init 构造的 typed client。
func Client() oauthclientservice.Client { return clientCli }

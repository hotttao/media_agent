// client.go - media_auth UserService typed client 装配 + 持久化 cookie middleware。
// 与 auth/team 共享同一 cookie store（.cookies/<host>/）。
package user

import (
	"log"

	userservice "media_agent/hertz_gen/media_auth/user_service"
	"media_agent/hertz_infra/clienthertz"
	config "media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
	"media_agent/media_cli/cmd/cookie"
)

var (
	clientCli userservice.Client
	store     *cookie.Store
)

func Init(cfg *config.Config) {
	cliCfg := cfg.GetClient()["media_auth"]
	if cliCfg == nil || !cliCfg.GetEnabled() {
		log.Fatalf("media_auth/user: client.media_auth 未配置或 enabled=false")
	}
	host := cliCfg.GetBaseDomain()
	if host == "" {
		log.Fatalf("media_auth/user: base_domain 为空")
	}
	base := clienthertz.NewClientSuiteFromConfig(cfg)
	appSuite, err := base.NewAppClientSuite("media_auth")
	if err != nil {
		log.Fatalf("media_auth/user: derive suite: %v", err)
	}
	shared, err := appSuite.NewClient()
	if err != nil {
		log.Fatalf("media_auth/user: build client: %v", err)
	}
	store = cookie.New(cmd.CookiesDir)
	clientCli, err = userservice.NewUserServiceClient(host,
		userservice.WithHertzClient(shared),
		userservice.WithHertzClientMiddleware(store.Middleware(host)),
	)
	if err != nil {
		log.Fatalf("media_auth/user: typed client: %v", err)
	}
}

func Client() userservice.Client { return clientCli }

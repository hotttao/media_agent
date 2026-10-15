package main

import (
	"log"
	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_plugin/biz/dal"
	repo "media_agent/media_plugin/biz/dal/repo/plugin_binding"
	accessHandler "media_agent/media_plugin/biz/handler/mcp_access"
	bindingHandler "media_agent/media_plugin/biz/handler/mcp_binding"
	"media_agent/media_plugin/biz/middleware"
	service "media_agent/media_plugin/biz/service/plugin_binding"
	"media_agent/media_plugin/biz/shared/client"
	"media_agent/media_plugin/biz/shared/secret"
	"media_agent/media_plugin/biz/workflow"
	"os"
)

func main() {
	loader, err := config.NewLoader()
	if err != nil {
		log.Fatal("load Plugin configuration: ", err)
	}
	defer loader.Close()
	cfg := loader.Current()
	key := os.Getenv("MCP_CREDENTIAL_ENCRYPTION_KEY")
	if key == "" || os.Getenv("MCP_CONTROL_CREDENTIAL") == "" || os.Getenv("MCP_GATEWAY_CREDENTIAL") == "" {
		log.Fatal("Plugin requires encryption key and internal service credentials")
	}
	db, err := dal.NewDB(cfg.GetDatabase())
	if err != nil {
		log.Fatal(err)
	}
	if db == nil {
		log.Fatal("Plugin database is required")
	}
	if err = dal.Migrate("migrations", cfg.GetDatabase()); err != nil {
		log.Fatal(err)
	}
	authority := &client.Authority{ControlURL: cfg.GetClient()["media-control"].GetBaseDomain(), GatewayURL: cfg.GetClient()["media-agent-gateway"].GetBaseDomain(), AuthURL: cfg.GetClient()["media-auth"].GetBaseDomain(), ControlToken: os.Getenv("CONTROL_INTERNAL_TOKEN"), MetadataCredential: os.Getenv("AGENT_METADATA_GATEWAY_CREDENTIAL"), AuthCredential: os.Getenv("MEDIA_AUTH_MCP_PLUGIN_TOKEN")}
	bindings := &workflow.Bindings{Service: service.New(repo.New(db), dal.NewTxRunner(db)), Authority: authority, Secrets: secret.New(key)}
	middleware.Configure(authority, os.Getenv("MCP_CONTROL_CREDENTIAL"), os.Getenv("MCP_GATEWAY_CREDENTIAL"))
	bindingHandler.Configure(bindings)
	accessHandler.Configure(bindings)
	suite := serverhertz.NewServerSuite(loader)
	h := suite.NewServer()
	register(h)
	suite.RegisterRoutes(h)
	h.Spin()
}

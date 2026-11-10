package main

import (
	"gorm.io/gorm"
	"media_agent/hertz_infra/authn/appcredential"
	"media_agent/media_control/biz/dal"
	metadatarepo "media_agent/media_control/biz/dal/repo/agent_configuration"
	repo "media_agent/media_control/biz/dal/repo/workspace"
	connections "media_agent/media_control/biz/handler/model_connection"
	handler "media_agent/media_control/biz/handler/workspace"
	metadata "media_agent/media_control/biz/service/agent_configuration"
	service "media_agent/media_control/biz/service/workspace"
	flow "media_agent/media_control/biz/workflow/application_agent"
)

func initApplicationAgents(db *gorm.DB, workspaces *service.Service, authURL, authSecret string, metadataEnabled bool) {
	f := &flow.Flow{Workspaces: workspaces, Bindings: service.NewApplicationBindings(repo.New(db)), Tx: dal.NewTxRunner(db)}
	if metadataEnabled {
		f.Metadata = metadata.New(metadatarepo.New(db), dal.NewTxRunner(db))
	}
	authority := appcredential.New(authURL, authSecret)
	handler.ConfigureApplications(f, authority)
	connections.ConfigureApplications(authority)
}

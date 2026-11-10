package main

import (
	"context"
	"gorm.io/gorm"
	"media_agent/media_auth/biz/dal"
	apprepo "media_agent/media_auth/biz/dal/repo/application"
	authrepo "media_agent/media_auth/biz/dal/repo/auth"
	identityrepo "media_agent/media_auth/biz/dal/repo/identity"
	orgrepo "media_agent/media_auth/biz/dal/repo/organization"
	teamrepo "media_agent/media_auth/biz/dal/repo/team"
	handler "media_agent/media_auth/biz/handler/application"
	internalhandler "media_agent/media_auth/biz/handler/application_internal"
	"media_agent/media_auth/biz/middleware"
	appservice "media_agent/media_auth/biz/service/application"
	authservice "media_agent/media_auth/biz/service/auth"
	identityservice "media_agent/media_auth/biz/service/identity"
	teamservice "media_agent/media_auth/biz/service/team"
	"media_agent/media_auth/biz/workflow"
	"os"
)

// initApplications is a composition root only; domain orchestration stays in workflow.
func initApplications(db *gorm.DB) {
	handler.Service = nil
	handler.Users = nil
	if db != nil {
		tx := dal.NewTxRunner(db)
		repo := apprepo.NewRepo(db)
		teams := teamservice.NewService(teamrepo.NewRepo(db), tx, orgrepo.NewRepo(db))
		s := appservice.NewService(repo, tx).WithBusiness(repo, applicationTeamPermissions{teams})
		internalhandler.Service = s
		internalhandler.InternalSecret = os.Getenv("MEDIA_AUTH_INTERNAL_TOKEN")
		handler.Service = s
		handler.Users = workflow.NewApplicationUsers(s, authservice.NewManagedUsers(authrepo.NewManagedRepo(db)), identityservice.NewService(identityrepo.NewRepo(db)), tx)
	}
	SessionMiddleware = middleware.ApplicationProtocol(SessionMiddleware)
}

type applicationTeamPermissions struct{ service *teamservice.Service }

func (a applicationTeamPermissions) Allowed(ctx context.Context, user, team, permission string) (string, bool, error) {
	v, e := a.service.Authorize(ctx, user, team, permission)
	return v.OrganizationID, v.Allowed && v.TeamRole != "" && v.OrganizationRole != "", e
}

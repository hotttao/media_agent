package main

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	workspacehandler "media_agent/media_control/biz/handler/workspace"
	"media_agent/media_control/biz/middleware"
)

func customizedRegister(r *server.Hertz) {
	r.POST("/internal/workspaces:check", middleware.Session(), middleware.RequireInternal(), workspacehandler.CheckWorkspaceAccess)
}

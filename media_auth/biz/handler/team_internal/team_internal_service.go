// Package team_internal 暴露 internal teams 查询，供下游服务（media_materials / media_workflow）
// 的 auth middleware fetchTeams 调用。
//
// 不挂网关、不出 OpenAPI；网络隔离 / mTLS 由部署层负责（反代 / service mesh / 网络策略）。
// 复用 team domain 的 ListMyTeams，不新建 domain / repo。
package team_internal

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	teamservice "media_agent/media_auth/biz/service/team"
)

// TeamSvc 由 wire_gen.go InitDI() 注入（复用 team domain service）。
var TeamSvc *teamservice.Service

// ListUserTeams POST /internal/teams:byUser -- 返回 user 所属 team id 列表。
//
// 供下游服务 auth middleware fetchTeams 调用；下游靠网络隔离 / opaque Bearer service token
// 访问，不经 cookie/session（在 session skip list）。
func ListUserTeams(ctx context.Context, c *app.RequestContext) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(c.Request.Body(), &req); err != nil {
		c.String(consts.StatusBadRequest, err.Error())
		return
	}
	if req.UserID == "" {
		c.String(consts.StatusBadRequest, "user_id required")
		return
	}
	teams, err := TeamSvc.ListMyTeams(ctx, req.UserID)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	ids := make([]string, 0, len(teams))
	for _, t := range teams {
		ids = append(ids, t.Team.ID)
	}
	c.JSON(consts.StatusOK, map[string]any{"team_ids": ids})
}

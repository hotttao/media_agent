// Package team_internal 暴露 internal teams 查询，供下游服务（media_materials / media_workflow）
// 的 auth middleware fetchTeams 调用。
//
// 不挂网关、不出 OpenAPI；网络隔离 / mTLS 由部署层负责（反代 / service mesh / 网络策略）。
// 复用 team domain 的 ListMyTeams，不新建 domain / repo。
package team_internal

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"os"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	teamservice "media_agent/media_auth/biz/service/team"
	ketoclient "media_agent/media_auth/biz/shared/client/keto"
)

// TeamSvc 由 wire_gen.go InitDI() 注入（复用 team domain service）。
var TeamSvc *teamservice.Service
var KetoClient *ketoclient.Client
var KetoEnforce bool

// ListUserTeams POST /internal/teams:byUser -- 返回 user 所属 team id 列表。
//
// 供下游服务 auth middleware fetchTeams 调用；下游靠网络隔离 / opaque Bearer service token
// 访问，不经 cookie/session（在 session skip list）。
func ListUserTeams(ctx context.Context, c *app.RequestContext) {
	if !authorizeInternal(c) {
		return
	}
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

// AuthorizeTeam POST /internal/team-authorization/check evaluates current facts and,
// when configured, the Keto projection. Enforce mode requires both decisions
// to allow so stale projection data can never override a synchronous revoke.
func AuthorizeTeam(ctx context.Context, c *app.RequestContext) {
	if !authorizeInternal(c) {
		return
	}
	var req struct {
		UserID     string `json:"user_id"`
		TeamID     string `json:"team_id"`
		Permission string `json:"permission"`
	}
	if err := json.Unmarshal(c.Request.Body(), &req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.UserID == "" || req.TeamID == "" || req.Permission == "" {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "user_id, team_id and permission are required"})
		return
	}
	facts, err := TeamSvc.Authorize(ctx, req.UserID, req.TeamID, req.Permission)
	if err != nil {
		if err == teamservice.ErrInvalidPermission {
			c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	allowed := facts.Allowed
	var ketoAllowed *bool
	if KetoClient != nil {
		decision, checkErr := KetoClient.Check(ctx, ketoclient.CheckRequest{
			Namespace: "Team", Object: req.TeamID, Relation: req.Permission, SubjectID: req.UserID,
		})
		if checkErr != nil {
			if KetoEnforce {
				c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "authorization projection unavailable"})
				return
			}
			log.Printf("authorization shadow check failed team_id=%s permission=%s error=%v", req.TeamID, req.Permission, checkErr)
		} else {
			ketoAllowed = &decision
			if decision != facts.Allowed {
				log.Printf("authorization shadow mismatch team_id=%s permission=%s fact_allowed=%t keto_allowed=%t", req.TeamID, req.Permission, facts.Allowed, decision)
			}
			if KetoEnforce {
				allowed = facts.Allowed && decision
			}
		}
	}
	c.JSON(consts.StatusOK, map[string]any{
		"allowed": allowed, "fact_allowed": facts.Allowed, "keto_allowed": ketoAllowed,
		"team_id": facts.TeamID, "organization_id": facts.OrganizationID,
		"team_role": facts.TeamRole, "organization_role": facts.OrganizationRole,
		"effective_feature_keys": facts.EffectiveFeatureKeys,
	})
}

func authorizeInternal(c *app.RequestContext) bool {
	expected := os.Getenv("MEDIA_AUTH_INTERNAL_TOKEN")
	actual := c.Request.Header.Peek("X-Media-Auth-Internal-Token")
	if expected == "" || subtle.ConstantTimeCompare(actual, []byte(expected)) != 1 {
		c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid internal token"})
		return false
	}
	return true
}

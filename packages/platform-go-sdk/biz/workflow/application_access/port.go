package application_access

import (
	"context"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	agent "media_agent/packages/platform-go-sdk/biz/workflow/agent_access"
	session "media_agent/packages/platform-go-sdk/biz/workflow/session_relay"
)

type API interface {
	agent.API
	session.API
	Me(context.Context, string) (contract.UserProfile, error)
	Exchange(context.Context, string, string, string, []string) (contract.Token, error)
}

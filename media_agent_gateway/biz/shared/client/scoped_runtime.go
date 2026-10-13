package client

import (
	"media_agent/hertz_infra/runtimeauth"
	d "media_agent/media_agent_gateway/biz/domain/agent_instance"
	"net/http"
	"time"
)

type ScopedRuntimeFactory struct {
	Signer *runtimeauth.Signer
	HTTP   *http.Client
}

func (f ScopedRuntimeFactory) ForInstance(instance d.Instance, epoch int64) ControlledRuntime {
	httpClient := f.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &HTTPRuntime{AgentType: instance.AgentType, BaseURL: instance.Endpoint, HTTP: httpClient, Signer: f.Signer, Scope: runtimeauth.Scope{WorkspaceID: instance.WorkspaceID, AgentConfigID: instance.AgentConfigID, ReplicaID: instance.ReplicaID, IncarnationID: instance.IncarnationID, ExecutionEpoch: epoch}}
}

package client

import (
	"context"
	"fmt"
	"net/http"

	gateway "media_agent/hertz_gen/media_agent_gateway/teamwork_runtime_service"
	dto "media_agent/hertz_gen/model/media_agent_gateway/teamwork_runtime"
)

type TeamworkEventPublisher struct {
	client gateway.Client
}

func NewTeamworkEventPublisher(baseURL, serviceToken string) (*TeamworkEventPublisher, error) {
	client, err := gateway.NewTeamworkRuntimeServiceClient(baseURL, gateway.WithHeader(http.Header{"X-Control-Service-Token": []string{serviceToken}}))
	if err != nil {
		return nil, err
	}
	return &TeamworkEventPublisher{client: client}, nil
}

func (p *TeamworkEventPublisher) Publish(ctx context.Context, eventID, workspaceID, topologyID string, version int64, status string) error {
	_, response, err := p.client.ApplyTopologyChanged(ctx, &dto.ApplyTopologyChangedRequest{EventId: eventID, WorkspaceId: workspaceID, TopologyId: topologyID, TopologyVersion: version, Status: status})
	if err != nil {
		return err
	}
	if response == nil || response.StatusCode() < 200 || response.StatusCode() >= 300 {
		if response == nil {
			return fmt.Errorf("gateway teamwork event returned no response")
		}
		return fmt.Errorf("gateway teamwork event returned HTTP %d", response.StatusCode())
	}
	return nil
}

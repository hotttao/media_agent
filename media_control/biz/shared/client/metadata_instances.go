package client

import (
	"context"
	"encoding/json"
	"errors"
	d "media_agent/media_control/biz/domain/agent_configuration"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type MetadataInstances struct {
	base, token string
	http        *http.Client
}

func NewMetadataInstances(base, token string) *MetadataInstances {
	return &MetadataInstances{strings.TrimRight(base, "/"), token, &http.Client{Timeout: 5 * time.Second}}
}
func (c *MetadataInstances) ListMetadataInstances(ctx context.Context, ws, user string) ([]d.Instance, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/internal/agent-metadata/instances?workspace_id="+url.QueryEscape(ws), nil)
	if err != nil {
		return nil, false, err
	}
	if c.token == "" {
		return nil, false, errors.New("instance membership unavailable")
	}
	req.Header.Set("X-Control-Service-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, false, errors.New("instance membership unavailable")
	}
	var value struct {
		Instances []struct {
			WorkspaceID   string `json:"workspace_id"`
			AgentConfigID string `json:"agent_config_id"`
			ReplicaID     string `json:"replica_id"`
			IncarnationID string `json:"incarnation_id"`
			Admission     string `json:"admission"`
		} `json:"instances"`
		Complete bool `json:"membership_complete"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&value); err != nil {
		return nil, false, err
	}
	out := []d.Instance{}
	for _, v := range value.Instances {
		if v.WorkspaceID != ws {
			return nil, false, errors.New("invalid instance membership")
		}
		out = append(out, d.Instance{WorkspaceID: v.WorkspaceID, AgentConfigID: v.AgentConfigID, ReplicaID: v.ReplicaID, IncarnationID: v.IncarnationID, Admission: v.Admission})
	}
	return out, value.Complete, nil
}

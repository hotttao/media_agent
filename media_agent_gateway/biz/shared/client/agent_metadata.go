package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrMetadataInstanceUnverified = errors.New("unverified runtime instance")

type MetadataResource struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Version    int64  `json:"version"`
	Name       string `json:"name"`
	Checksum   string `json:"checksum"`
	ContentRef string `json:"content_ref"`
	Content    string `json:"content"`
}
type MetadataContext struct {
	MetadataSchemaVersion         int                `json:"metadata_schema_version"`
	WorkspaceID                   string             `json:"workspace_id"`
	AgentConfigID                 string             `json:"agent_config_id"`
	SelectionRevision             int64              `json:"selection_revision"`
	TargetContextVersion          string             `json:"target_context_version"`
	ContextVersion                int64              `json:"context_version"`
	AgentType                     string             `json:"agent_type"`
	ConfiguredPrompt              string             `json:"configured_prompt"`
	Resources                     []MetadataResource `json:"resources"`
	Status                        string             `json:"status"`
	PlatformPrompt                string             `json:"platform_prompt"`
	PlatformPromptTemplateVersion string             `json:"platform_prompt_template_version"`
}
type MetadataResult struct {
	ReportID              string `json:"report_id"`
	WorkspaceID           string `json:"workspace_id"`
	AgentConfigID         string `json:"agent_config_id"`
	SelectionRevision     int64  `json:"selection_revision"`
	TargetContextVersion  string `json:"target_context_version"`
	ReplicaID             string `json:"replica_id"`
	IncarnationID         string `json:"incarnation_id"`
	Status                string `json:"status"`
	AppliedContextVersion string `json:"applied_context_version"`
	ErrorCode             string `json:"error_code"`
}
type MetadataInstance struct {
	WorkspaceID   string `json:"workspace_id"`
	AgentConfigID string `json:"agent_config_id"`
	ReplicaID     string `json:"replica_id"`
	IncarnationID string `json:"incarnation_id"`
	Admission     string `json:"admission"`
}
type MetadataQuery struct {
	WorkspaceID, AgentConfigID, ReplicaID, IncarnationID, TargetContextVersion string
	SelectionRevision                                                          int64
	ForSession                                                                 bool
}
type MetadataArtifact struct {
	Content  []byte `json:"content"`
	Checksum string `json:"checksum"`
}
type MetadataHTTPError struct{ Status int }

func (e MetadataHTTPError) Error() string { return "metadata upstream rejected request" }

type MetadataControlClient struct {
	base, token, metadataCredential string
	http                            *http.Client
}

func NewMetadataControlClient(base, token, metadataCredential string) *MetadataControlClient {
	return &MetadataControlClient{strings.TrimRight(base, "/"), token, metadataCredential, &http.Client{Timeout: 10 * time.Second}}
}
func (c *MetadataControlClient) do(ctx context.Context, user, method, path string, body, out any) error {
	if c.token == "" || c.metadataCredential == "" {
		return errors.New("metadata authority unavailable")
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-Control-Service-Token", c.token)
	req.Header.Set("X-Metadata-Gateway-Credential", c.metadataCredential)
	req.Header.Set("X-Control-Acting-User-ID", user)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("metadata authority unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return MetadataHTTPError{resp.StatusCode}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 12<<20)).Decode(out)
}
func (c *MetadataControlClient) Get(ctx context.Context, user string, q MetadataQuery) (MetadataContext, error) {
	values := url.Values{"agent_config_id": {q.AgentConfigID}, "target_context_version": {q.TargetContextVersion}}
	if q.SelectionRevision > 0 {
		values.Set("selection_revision", jsonNumber(q.SelectionRevision))
	}
	if q.ForSession {
		values.Set("for_session", "true")
	}
	var out MetadataContext
	err := c.do(ctx, user, "GET", "/internal/workspaces/"+url.PathEscape(q.WorkspaceID)+"/agent-metadata?"+values.Encode(), nil, &out)
	return out, err
}
func jsonNumber(n int64) string { body, _ := json.Marshal(n); return string(body) }
func (c *MetadataControlClient) Report(ctx context.Context, user string, v MetadataResult) error {
	var out struct{ Accepted bool }
	err := c.do(ctx, user, "POST", "/internal/workspaces/"+url.PathEscape(v.WorkspaceID)+"/agent-metadata/results", map[string]any{"result": v}, &out)
	if err == nil && !out.Accepted {
		return errors.New("metadata report not accepted")
	}
	return err
}
func (c *MetadataControlClient) Artifact(ctx context.Context, user string, q MetadataQuery, checksum string) (MetadataArtifact, error) {
	values := url.Values{"agent_config_id": {q.AgentConfigID}, "target_context_version": {q.TargetContextVersion}}
	var out MetadataArtifact
	err := c.do(ctx, user, "GET", "/internal/workspaces/"+url.PathEscape(q.WorkspaceID)+"/agent-metadata/artifacts/"+url.PathEscape(checksum)+"?"+values.Encode(), nil, &out)
	return out, err
}

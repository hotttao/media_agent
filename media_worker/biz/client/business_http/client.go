// Package business_http is the transport used only by the media management APIs.
package business_http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"media_agent/hertz_infra/authn/mcpdelegation"
	contract "media_agent/media_worker/biz/shared/media_management"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL, ServiceToken, Audience string
	HTTP                            *http.Client
	timeout                         time.Duration
}

func New(base, token, audience string) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") || len(token) < 32 {
		return nil, &contract.Failure{Code: "configuration_invalid", Message: "Invalid business client configuration"}
	}
	if audience != mcpdelegation.Workflow && audience != mcpdelegation.Materials {
		return nil, mcpdelegation.ErrInvalid
	}
	return &Client{BaseURL: strings.TrimRight(base, "/"), ServiceToken: token, Audience: audience, timeout: 10 * time.Second, HTTP: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Request(ctx context.Context, method, path string, query url.Values, body any) (map[string]any, error) {
	if ctx.Err() != nil {
		return nil, &contract.Failure{Code: "not_dispatched", Message: "Request deadline reached", NotDispatched: true, Stop: true}
	}
	credentials := contract.CredentialsFromContext(ctx)
	token := credentials.Workflow
	if c.Audience == mcpdelegation.Materials {
		token = credentials.Materials
	}
	if token == "" {
		return nil, &contract.Failure{Code: "permission_denied", Message: "Business module access is required", NotDispatched: true}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, &contract.Failure{Code: "invalid_argument", Message: "Invalid business request", NotDispatched: true}
	}
	if body == nil {
		raw = nil
	}
	address := c.BaseURL + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	batchCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(raw))
	if err != nil {
		return nil, &contract.Failure{Code: "not_dispatched", Message: "Invalid business address", NotDispatched: true}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(mcpdelegation.ServiceHeader, c.ServiceToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &contract.Failure{Code: "upstream_unavailable", Message: "Business response unavailable", Unknown: method != "GET", Stop: batchCtx.Err() != nil}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		failure := &contract.Failure{Code: "operation_failed", Message: "Business API rejected the request"}
		switch response.StatusCode {
		case 401:
			failure.Code = "authentication_failed"
			failure.Stop = true
		case 403, 404:
			failure.Code = "not_found_or_forbidden"
		case 409:
			failure.Code = "conflict"
			switch response.Header.Get("X-Media-Workflow-Error") {
			case "version_conflict":
				failure.Code = "version_conflict"
			case "idempotency_conflict":
				failure.Code = "idempotency_conflict"
			}
		case 400, 422:
			failure.Code = "invalid_argument"
		default:
			if response.StatusCode >= 500 {
				failure.Code = "upstream_unavailable"
				failure.Unknown = method != "GET"
			}
		}
		if response.Header.Get("X-Media-MCP-Admission") == "rejected" {
			failure.Stop = true
			failure.Unknown = false
			failure.NotDispatched = true
		}
		return nil, failure
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return nil, &contract.Failure{Code: "outcome_unknown", Message: "Business response incomplete", Unknown: method != "GET"}
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil || result == nil {
		return nil, &contract.Failure{Code: "outcome_unknown", Message: "Invalid business response", Unknown: method != "GET"}
	}
	return result, nil
}

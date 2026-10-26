package worker_gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"media_agent/media_worker/biz/shared/correlation"
	"media_agent/media_worker/biz/shared/worker_runtime"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type HTTPError struct {
	Method, Path string
	StatusCode   int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("worker gateway %s %s: status=%d", e.Method, e.Path, e.StatusCode)
}

func New(baseURL, apiToken string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"), token: apiToken,
		http: &http.Client{Timeout: 35 * time.Second},
	}
}

type capabilityDTO struct {
	Key         string `json:"capability_key"`
	Version     int    `json:"capability_version"`
	Concurrency int    `json:"concurrency"`
}

type taskDTO struct {
	DispatchID        string `json:"dispatch_id"`
	Attempt           int    `json:"attempt"`
	CapabilityKey     string `json:"capability_key"`
	CapabilityVersion int    `json:"capability_version"`
	InputJSON         string `json:"input_json"`
	LeaseExpiresAt    int64  `json:"lease_expires_at"`
	TraceParent       string `json:"trace_parent,omitempty"`
}

type pollResponse struct {
	Task *taskDTO `json:"task"`
}

type receiptDTO struct {
	ReceiptID  string `json:"receipt_id"`
	DispatchID string `json:"dispatch_id"`
	Status     string `json:"status"`
}

func (c *Client) Register(ctx context.Context, workerID string, capabilities []worker_runtime.Capability) error {
	body := map[string]any{"worker_instance_id": workerID, "capabilities": capabilityDTOs(capabilities)}
	return c.doJSON(ctx, http.MethodPost, "/api/worker/v1/registrations", body, nil)
}

func (c *Client) Poll(ctx context.Context, workerID string, capability worker_runtime.Capability) (*worker_runtime.Task, error) {
	body := map[string]any{
		"worker_instance_id": workerID, "capability_key": capability.Key, "capability_version": capability.Version,
	}
	var response pollResponse
	status, err := c.doJSONStatus(ctx, http.MethodPost, "/api/worker/v1/tasks:poll", body, &response)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNoContent || response.Task == nil {
		return nil, nil
	}
	t := response.Task
	input := map[string]any(nil)
	if t.InputJSON != "" {
		if json.Unmarshal([]byte(t.InputJSON), &input) != nil {
			return nil, errors.New("decode task input")
		}
	}
	parent, _ := input["_traceparent"].(string)
	if !correlation.Valid(parent) {
		parent = ""
	}
	delete(input, "_traceparent")
	return &worker_runtime.Task{DispatchID: t.DispatchID, Attempt: t.Attempt, CapabilityKey: t.CapabilityKey, CapabilityVersion: t.CapabilityVersion, Input: input, LeaseExpiresAt: t.LeaseExpiresAt, TraceParent: parent}, nil
}

func (c *Client) Heartbeat(ctx context.Context, workerID string, task worker_runtime.Task) error {
	ctx = correlation.WithParent(ctx, task.TraceParent)
	body := map[string]any{"worker_instance_id": workerID, "attempt": task.Attempt}
	path := "/api/worker/v1/tasks/" + url.PathEscape(task.DispatchID) + "/heartbeat"
	return c.doJSON(ctx, http.MethodPost, path, body, nil)
}

func (c *Client) Complete(ctx context.Context, workerID string, task worker_runtime.Task, idempotencyKey string, output map[string]any) (*worker_runtime.ResultReceipt, error) {
	ctx = correlation.WithParent(ctx, task.TraceParent)
	encoded, err := json.Marshal(output)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"worker_instance_id": workerID, "attempt": task.Attempt,
		"idempotency_key": idempotencyKey, "output_json": string(encoded),
	}
	path := "/api/worker/v1/tasks/" + url.PathEscape(task.DispatchID) + "/complete"
	var receipt receiptDTO
	if err := c.doJSON(ctx, http.MethodPost, path, body, &receipt); err != nil {
		return nil, err
	}
	if receipt.Status != "submitted" {
		return nil, errors.New("native task result is not confirmed")
	}
	return &worker_runtime.ResultReceipt{ReceiptID: receipt.ReceiptID, DispatchID: receipt.DispatchID, Status: receipt.Status}, nil
}

func (c *Client) Fail(ctx context.Context, workerID string, task worker_runtime.Task, idempotencyKey, reason string) (*worker_runtime.ResultReceipt, error) {
	return c.FailWithKind(ctx, workerID, task, idempotencyKey, reason, "permanent")
}

func (c *Client) FailWithKind(ctx context.Context, workerID string, task worker_runtime.Task, idempotencyKey, reason, kind string) (*worker_runtime.ResultReceipt, error) {
	ctx = correlation.WithParent(ctx, task.TraceParent)
	body := map[string]any{
		"worker_instance_id": workerID, "attempt": task.Attempt,
		"idempotency_key": idempotencyKey, "reason": reason,
		"failure_kind": kind,
	}
	path := "/api/worker/v1/tasks/" + url.PathEscape(task.DispatchID) + "/fail"
	var receipt receiptDTO
	if err := c.doJSON(ctx, http.MethodPost, path, body, &receipt); err != nil {
		return nil, err
	}
	if receipt.Status != "submitted" {
		return nil, errors.New("native task result is not confirmed")
	}
	return &worker_runtime.ResultReceipt{ReceiptID: receipt.ReceiptID, DispatchID: receipt.DispatchID, Status: receipt.Status}, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body, output any) error {
	_, err := c.doJSONStatus(ctx, method, path, body, output)
	return err
}

func (c *Client) doJSONStatus(ctx context.Context, method, path string, body, output any) (int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	if parent := correlation.Parent(ctx); parent != "" {
		request.Header.Set("traceparent", parent)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Gateway errors may contain task inputs or credentials; do not retain
		// their response bodies in errors that the client logs during retries.
		if response.StatusCode == 409 || response.StatusCode == 410 {
			return response.StatusCode, worker_runtime.ErrResultRejected
		}
		return response.StatusCode, &HTTPError{Method: method, Path: path, StatusCode: response.StatusCode}
	}
	if output != nil && response.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(response.Body).Decode(output); err != nil {
			return response.StatusCode, err
		}
	}
	return response.StatusCode, nil
}

func capabilityDTOs(caps []worker_runtime.Capability) []capabilityDTO {
	out := make([]capabilityDTO, len(caps))
	for i, c := range caps {
		out[i] = capabilityDTO{Key: c.Key, Version: c.Version, Concurrency: c.Concurrency}
	}
	return out
}

var _ worker_runtime.Gateway = (*Client)(nil)
var _ worker_runtime.Uploader = (*Client)(nil)

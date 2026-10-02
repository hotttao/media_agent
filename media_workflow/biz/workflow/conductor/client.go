package conductor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"media_agent/media_workflow/biz/workflow/compiler"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 35 * time.Second}}
}

func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" }

type TaskDefinition struct {
	Name                   string `json:"name"`
	Description            string `json:"description,omitempty"`
	RetryCount             int    `json:"retryCount"`
	RetryLogic             string `json:"retryLogic,omitempty"`
	RetryDelaySeconds      int    `json:"retryDelaySeconds,omitempty"`
	TimeoutSeconds         int    `json:"timeoutSeconds,omitempty"`
	ResponseTimeoutSeconds int    `json:"responseTimeoutSeconds,omitempty"`
	TimeoutPolicy          string `json:"timeoutPolicy,omitempty"`
}

func (c *Client) RegisterTaskDefinitions(ctx context.Context, definitions []TaskDefinition) error {
	if len(definitions) == 0 {
		return nil
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/metadata/taskdefs", definitions, nil); err == nil {
		return nil
	}
	for _, definition := range definitions {
		if err := c.doJSON(ctx, http.MethodPut, "/api/metadata/taskdefs", definition, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) RegisterWorkflow(ctx context.Context, definition compiler.Definition) error {
	var response struct {
		Errors map[string]string `json:"bulkErrorResults"`
	}
	if err := c.doJSON(ctx, http.MethodPut, "/api/metadata/workflow", []compiler.Definition{definition}, &response); err != nil {
		return err
	}
	if len(response.Errors) > 0 {
		return fmt.Errorf("conductor workflow registration failed: %v", response.Errors)
	}
	return nil
}

type StartWorkflowRequest struct {
	Name                string            `json:"name"`
	Version             int               `json:"version"`
	CorrelationID       string            `json:"correlationId,omitempty"`
	IdempotencyKey      string            `json:"idempotencyKey,omitempty"`
	IdempotencyStrategy string            `json:"idempotencyStrategy,omitempty"`
	Input               map[string]any    `json:"input"`
	TaskToDomain        map[string]string `json:"taskToDomain,omitempty"`
}

func (c *Client) StartWorkflow(ctx context.Context, request StartWorkflowRequest) (string, error) {
	var id string
	if err := c.doJSON(ctx, http.MethodPost, "/api/workflow", request, &id); err != nil {
		return "", fmt.Errorf("conductor start workflow: %w", err)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errorsNew("conductor returned an empty workflow id")
	}
	return id, nil
}

type Task struct {
	TaskType           string         `json:"taskType"`
	ReferenceTaskName  string         `json:"referenceTaskName"`
	TaskID             string         `json:"taskId"`
	WorkflowInstanceID string         `json:"workflowInstanceId"`
	Status             string         `json:"status"`
	InputData          map[string]any `json:"inputData"`
	OutputData         map[string]any `json:"outputData"`
	RetryCount         int            `json:"retryCount"`
}

func (c *Client) Poll(ctx context.Context, taskType, workerID, domain string) (*Task, error) {
	// OSS batch polling accepts milliseconds, with a hard maximum of 5000.
	values := url.Values{"workerid": []string{workerID}, "count": []string{"1"}, "timeout": []string{"5000"}}
	if domain != "" {
		values.Set("domain", domain)
	}
	path := fmt.Sprintf("/api/tasks/poll/batch/%s?%s", url.PathEscape(taskType), values.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("conductor poll %s: status=%d body=%s", taskType, response.StatusCode, body)
	}
	var tasks []Task
	if err := json.NewDecoder(response.Body).Decode(&tasks); err != nil {
		return nil, err
	}
	if len(tasks) == 0 || tasks[0].TaskID == "" {
		return nil, nil
	}
	return &tasks[0], nil
}

type TaskResult struct {
	WorkflowInstanceID    string         `json:"workflowInstanceId"`
	TaskID                string         `json:"taskId"`
	WorkerID              string         `json:"workerId,omitempty"`
	Status                string         `json:"status"`
	OutputData            map[string]any `json:"outputData,omitempty"`
	ReasonForIncompletion string         `json:"reasonForIncompletion,omitempty"`
	CallbackAfterSeconds  int            `json:"callbackAfterSeconds,omitempty"`
}

func (c *Client) UpdateTask(ctx context.Context, result TaskResult) error {
	return c.doJSON(ctx, http.MethodPost, "/api/tasks", result, nil)
}

func (c *Client) CompleteTaskByReference(ctx context.Context, workflowID, taskReferenceName string, output map[string]any) error {
	path := fmt.Sprintf("/api/tasks/%s/%s/COMPLETED?workerid=media-workflow-api", url.PathEscape(workflowID), url.PathEscape(taskReferenceName))
	return c.doJSON(ctx, http.MethodPost, path, output, nil)
}

func (c *Client) RerunFromTask(ctx context.Context, workflowID, taskID string) error {
	request := map[string]any{"reRunFromWorkflowId": workflowID, "reRunFromTaskId": taskID}
	return c.doJSON(ctx, http.MethodPost, "/api/workflow/"+url.PathEscape(workflowID)+"/rerun", request, nil)
}

func (c *Client) RerunWorkflow(ctx context.Context, workflowID string, input map[string]any) error {
	request := map[string]any{"reRunFromWorkflowId": workflowID, "workflowInput": input}
	return c.doJSON(ctx, http.MethodPost, "/api/workflow/"+url.PathEscape(workflowID)+"/rerun", request, nil)
}

func (c *Client) TerminateWorkflow(ctx context.Context, workflowID, reason string) error {
	path := "/api/workflow/" + url.PathEscape(workflowID)
	if reason != "" {
		path += "?reason=" + url.QueryEscape(reason)
	}
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

type WorkflowExecution struct {
	WorkflowID     string         `json:"workflowId"`
	CorrelationID  string         `json:"correlationId"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Status         string         `json:"status"`
	Output         map[string]any `json:"output"`
	Tasks          []Task         `json:"tasks"`
}

func (c *Client) GetByCorrelation(ctx context.Context, name, correlationID string) (string, error) {
	path := "/api/workflow/" + url.PathEscape(name) + "/correlated/" + url.PathEscape(correlationID) + "?includeClosed=true"
	var executions []WorkflowExecution
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &executions); err != nil {
		return "", err
	}
	var found string
	for _, execution := range executions {
		if execution.CorrelationID != correlationID || execution.WorkflowID == "" {
			continue
		}
		if found != "" && found != execution.WorkflowID {
			return "", fmt.Errorf("multiple Conductor executions share resume correlation %q", correlationID)
		}
		found = execution.WorkflowID
	}
	return found, nil
}

func (c *Client) GetWorkflow(ctx context.Context, id string) (*WorkflowExecution, error) {
	var execution WorkflowExecution
	if err := c.doJSON(ctx, http.MethodGet, "/api/workflow/"+url.PathEscape(id)+"?includeTasks=true", nil, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, input any, output any) error {
	if !c.Enabled() {
		return errorsNew("conductor is not configured")
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		return fmt.Errorf("conductor %s %s: status=%d body=%s", method, path, response.StatusCode, responseBody)
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if target, ok := output.(*string); ok {
		// Conductor OSS returns a raw UUID from workflow start even when
		// Content-Type is application/json. Other adapters return JSON strings.
		data, err := io.ReadAll(io.LimitReader(response.Body, 4096))
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, target); err == nil {
			return nil
		}
		id := strings.TrimSpace(string(data))
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("conductor returned an invalid workflow id")
		}
		*target = id
		return nil
	}
	return json.NewDecoder(response.Body).Decode(output)
}

func errorsNew(message string) error { return fmt.Errorf("%s", message) }

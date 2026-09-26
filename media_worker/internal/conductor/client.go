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
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 35 * time.Second}}
}

type Task struct {
	TaskType           string         `json:"taskType"`
	ReferenceTaskName  string         `json:"referenceTaskName"`
	TaskID             string         `json:"taskId"`
	WorkflowInstanceID string         `json:"workflowInstanceId"`
	InputData          map[string]any `json:"inputData"`
}

func (c *Client) Poll(ctx context.Context, taskType, workerID string) (*Task, error) {
	path := fmt.Sprintf("/api/tasks/poll/%s?workerid=%s", url.PathEscape(taskType), url.QueryEscape(workerID))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("poll %s: status=%d body=%s", taskType, response.StatusCode, body)
	}
	var task Task
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		return nil, err
	}
	if task.TaskID == "" {
		return nil, nil
	}
	return &task, nil
}

type Result struct {
	WorkflowInstanceID    string         `json:"workflowInstanceId"`
	TaskID                string         `json:"taskId"`
	WorkerID              string         `json:"workerId"`
	Status                string         `json:"status"`
	OutputData            map[string]any `json:"outputData,omitempty"`
	ReasonForIncompletion string         `json:"reasonForIncompletion,omitempty"`
}

func (c *Client) Update(ctx context.Context, result Result) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/tasks", bytes.NewReader(body))
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
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("update task: status=%d body=%s", response.StatusCode, responseBody)
	}
	return nil
}

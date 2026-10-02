package conductor

import (
	"context"
	"encoding/json"
	"fmt"

	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/workflow/compiler"
)

// StartResume replays the exact frozen outbox payload. Conductor's own
// RETURN_EXISTING key closes the crash window after a successful remote start.
func (c *Client) StartResume(ctx context.Context, start jobdomain.ResumeStart) (string, string, error) {
	var definition compiler.Definition
	if err := json.Unmarshal(start.DefinitionJSON, &definition); err != nil || definition.Name == "" || definition.Version <= 0 {
		return "", "", fmt.Errorf("invalid frozen resume definition")
	}
	var taskDomains map[string]string
	if err := json.Unmarshal(start.TaskDomainsJSON, &taskDomains); err != nil {
		return "", "", fmt.Errorf("invalid frozen resume task domains: %w", err)
	}
	var input map[string]any
	if err := json.Unmarshal(start.WorkflowInputJSON, &input); err != nil || input["job_id"] != start.JobID {
		return "", "", fmt.Errorf("invalid frozen resume workflow input")
	}
	if err := c.RegisterTaskDefinitions(ctx, []TaskDefinition{{
		Name: compiler.RestoreStepTaskName, Description: "Restore a verified historical StepRun",
		RetryCount: 3, RetryLogic: "FIXED", RetryDelaySeconds: 5, TimeoutSeconds: 300,
		ResponseTimeoutSeconds: 120, TimeoutPolicy: "TIME_OUT_WF",
	}}); err != nil {
		return "", "", fmt.Errorf("register historical restore task: %w", err)
	}
	if err := c.RegisterWorkflow(ctx, definition); err != nil {
		return "", "", err
	}
	hash, err := compiler.DefinitionHash(definition)
	if err != nil {
		return "", "", err
	}
	id, err := c.StartWorkflow(ctx, StartWorkflowRequest{
		Name: definition.Name, Version: definition.Version,
		CorrelationID:       start.CorrelationID,
		IdempotencyKey:      start.CorrelationID,
		IdempotencyStrategy: "RETURN_EXISTING",
		Input:               input, TaskToDomain: taskDomains,
	})
	if err != nil {
		// A timed-out Start response is ambiguous. Query the stable correlation
		// before retrying; the next attempt still uses RETURN_EXISTING.
		if existing, lookupErr := c.GetByCorrelation(ctx, definition.Name, start.CorrelationID); lookupErr == nil && existing != "" {
			return existing, hash, nil
		}
		return "", "", err
	}
	return id, hash, nil
}

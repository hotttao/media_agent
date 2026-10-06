package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"media_agent/media_workflow/biz/workflow/compiler"
)

var ErrWorkflowDefinitionConflict = errors.New("conductor workflow definition conflicts with frozen compilation")
var ErrWorkflowRegistrationUnavailable = errors.New("conductor workflow registration is not verified")

// EnsureWorkflow verifies compiler-owned fields, including the frozen template
// and capability inputs. Conductor audit/default fields are not compiler-owned.
// It never overwrites an observed conflicting version, and never blindly
// repeats a PUT after an uncertain response. This is not an upstream CAS API.
func (c *Client) EnsureWorkflow(ctx context.Context, definition compiler.Definition) error {
	if found, err := c.CheckWorkflowDefinition(ctx, definition); err != nil || found {
		return err
	}
	// A committed PUT with a lost response is recovered by this same exact
	// version lookup, not a second write or a lookup of the latest version.
	writeErr := c.RegisterWorkflow(ctx, definition)
	found, verifyErr := c.CheckWorkflowDefinition(ctx, definition)
	if verifyErr != nil {
		return verifyErr
	}
	if found {
		return nil
	}
	if writeErr != nil {
		return ErrWorkflowRegistrationUnavailable
	}
	return ErrWorkflowRegistrationUnavailable
}

// CheckWorkflowDefinition is a read-only preflight, also used before any task
// metadata or compilation persistence side effects in the publication flow.
func (c *Client) CheckWorkflowDefinition(ctx context.Context, definition compiler.Definition) (bool, error) {
	if definition.Name == "" || definition.Version <= 0 {
		return false, ErrWorkflowRegistrationUnavailable
	}
	expected, err := compiler.DefinitionHash(definition)
	if err != nil {
		return false, ErrWorkflowRegistrationUnavailable
	}
	var metadata map[string]any
	path := "/api/metadata/workflow/" + url.PathEscape(definition.Name) + "?version=" + fmt.Sprint(definition.Version)
	err = c.doJSON(ctx, http.MethodGet, path, nil, &metadata)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, ErrWorkflowRegistrationUnavailable
	}
	if !compatibleMetadata(metadata, false) {
		return false, ErrWorkflowDefinitionConflict
	}
	var actual compiler.Definition
	encoded, err := json.Marshal(metadata)
	if err != nil || json.Unmarshal(encoded, &actual) != nil {
		return false, ErrWorkflowRegistrationUnavailable
	}
	hash, err := compiler.DefinitionHash(actual)
	if err != nil {
		return false, ErrWorkflowRegistrationUnavailable
	}
	if hash != expected {
		return false, ErrWorkflowDefinitionConflict
	}
	return true, nil
}

// Reject execution-changing extensions rather than silently dropping them
// while decoding the compiler's subset. Audit timestamps and known engine
// defaults are permitted; unfamiliar nonempty fields fail closed.
func compatibleMetadata(metadata map[string]any, task bool) bool {
	owned := map[string]bool{"name": true, "description": true, "version": true, "tasks": true, "inputParameters": true, "outputParameters": true, "schemaVersion": true}
	if task {
		owned = map[string]bool{"name": true, "taskReferenceName": true, "type": true, "inputParameters": true, "subWorkflowParam": true, "joinOn": true, "forkTasks": true, "evaluatorType": true, "expression": true, "decisionCases": true, "loopOver": true, "loopCondition": true}
	}
	for key, value := range metadata {
		if owned[key] {
			if key == "subWorkflowParam" {
				if sub, ok := value.(map[string]any); ok {
					for field, setting := range sub {
						if field != "name" && field != "version" && !emptyMetadataValue(setting) {
							return false
						}
					}
				}
			}
			if key == "tasks" || key == "loopOver" {
				if !compatibleTaskList(value) {
					return false
				}
			}
			if key == "forkTasks" {
				if branches, ok := value.([]any); ok {
					for _, branch := range branches {
						if !compatibleTaskList(branch) {
							return false
						}
					}
				}
			}
			if key == "decisionCases" {
				if cases, ok := value.(map[string]any); ok {
					for _, tasks := range cases {
						if !compatibleTaskList(tasks) {
							return false
						}
					}
				}
			}
			continue
		}
		if !task && (key == "createTime" || key == "updateTime") {
			continue
		}
		if !task && (key == "restartable" || key == "enforceSchema") {
			if value != true {
				return false
			}
			continue
		}
		if !task && key == "timeoutPolicy" && value == "ALERT_ONLY" {
			continue
		}
		if !emptyMetadataValue(value) {
			return false
		}
	}
	return true
}

func compatibleTaskList(value any) bool {
	if value == nil {
		return true
	}
	list, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range list {
		task, ok := item.(map[string]any)
		if !ok || !compatibleMetadata(task, true) {
			return false
		}
	}
	return true
}

func emptyMetadataValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case bool:
		return !v
	case string:
		return v == ""
	case float64:
		return v == 0
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		return false
	}
}

package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

var ErrTaskDefinitionConflict = errors.New("conductor task definition conflicts with frozen capability")
var ErrTaskRegistrationUnavailable = errors.New("conductor task definition registration is not verified")

// Defaults mirror the locked Conductor TaskDef contract, not Go zero values.
// The fields tagged omitempty are omitted when zero and acquire engine defaults.
func normalizedTaskDefinition(d TaskDefinition) TaskDefinition {
	if d.RetryLogic == "" {
		d.RetryLogic = "FIXED"
	}
	if d.RetryDelaySeconds == 0 {
		d.RetryDelaySeconds = 60
	}
	if d.ResponseTimeoutSeconds == 0 {
		d.ResponseTimeoutSeconds = 3600
	}
	if d.TimeoutPolicy == "" {
		d.TimeoutPolicy = "TIME_OUT_WF"
	}
	return d
}

func (c *Client) checkTaskDefinition(ctx context.Context, definition TaskDefinition) (bool, error) {
	if definition.Name == "" {
		return false, ErrTaskRegistrationUnavailable
	}
	var metadata map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/api/metadata/taskdefs/"+url.PathEscape(definition.Name), nil, &metadata)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, ErrTaskRegistrationUnavailable
	}
	owned := map[string]bool{"name": true, "description": true, "retryCount": true, "retryLogic": true, "retryDelaySeconds": true, "timeoutSeconds": true, "responseTimeoutSeconds": true, "timeoutPolicy": true}
	for key, value := range metadata {
		if owned[key] {
			continue
		}
		switch key {
		case "createTime", "updateTime", "createdBy", "updatedBy":
			continue
		case "rateLimitFrequencyInSeconds", "backoffScaleFactor":
			if value == float64(1) {
				continue
			}
			return false, ErrTaskDefinitionConflict
		case "taskStatusListenerEnabled":
			if value == true {
				continue
			}
		}
		if !emptyMetadataValue(value) {
			return false, ErrTaskDefinitionConflict
		}
	}
	// This field defaults true, unlike other empty boolean extensions. A
	// disabled listener is not compatible with the default we registered.
	if value, exists := metadata["taskStatusListenerEnabled"]; exists && value != true {
		return false, ErrTaskDefinitionConflict
	}
	encoded, err := json.Marshal(metadata)
	var actual TaskDefinition
	if err != nil || json.Unmarshal(encoded, &actual) != nil {
		return false, ErrTaskRegistrationUnavailable
	}
	// Preserve explicit zero values returned by the engine; they are not the
	// same as omitted request fields acquiring defaults.
	if _, present := metadata["retryLogic"]; !present {
		actual.RetryLogic = "FIXED"
	}
	if _, present := metadata["retryDelaySeconds"]; !present {
		actual.RetryDelaySeconds = 60
	}
	if _, present := metadata["responseTimeoutSeconds"]; !present {
		actual.ResponseTimeoutSeconds = 3600
	}
	if _, present := metadata["timeoutPolicy"]; !present {
		actual.TimeoutPolicy = "TIME_OUT_WF"
	}
	if actual != normalizedTaskDefinition(definition) {
		return false, ErrTaskDefinitionConflict
	}
	return true, nil
}

// Read every name before writing anything. Existing matching definitions are
// shared immutable contracts; conflicts require a new capability/task name.
// One POST creates only missing definitions. Even when its response is lost,
// exact GETs verify acceptance. Never fall back to a destructive metadata PUT.
// Upstream offers no CAS; external metadata writers remain a separate concern.
func (c *Client) RegisterTaskDefinitions(ctx context.Context, definitions []TaskDefinition) error {
	unique := make([]TaskDefinition, 0, len(definitions))
	byName := map[string]TaskDefinition{}
	for _, definition := range definitions {
		if definition.Name == "" {
			return ErrTaskRegistrationUnavailable
		}
		if previous, ok := byName[definition.Name]; ok {
			if normalizedTaskDefinition(previous) != normalizedTaskDefinition(definition) {
				return ErrTaskDefinitionConflict
			}
			continue
		}
		byName[definition.Name] = definition
		unique = append(unique, definition)
	}
	missing := make([]TaskDefinition, 0, len(unique))
	for _, definition := range unique {
		found, err := c.checkTaskDefinition(ctx, definition)
		if err != nil {
			return err
		}
		if !found {
			missing = append(missing, definition)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	// Outcome is determined by exact metadata, not the ambiguous POST result.
	_ = c.doJSON(ctx, http.MethodPost, "/api/metadata/taskdefs", missing, nil)
	for _, definition := range unique {
		found, err := c.checkTaskDefinition(ctx, definition)
		if err != nil {
			return err
		}
		if !found {
			return ErrTaskRegistrationUnavailable
		}
	}
	return nil
}

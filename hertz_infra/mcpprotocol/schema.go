package mcpprotocol

import (
	"bytes"
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
)

// Validator keeps schema implementation types outside application packages.
type Validator struct{ schema *jsonschema.Resolved }

func CompileSchema(value any) (*Validator, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var schema jsonschema.Schema
	if err = json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, err
	}
	return &Validator{resolved}, nil
}
func (v *Validator) Validate(raw []byte) error {
	var value any
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := d.Decode(&value); err != nil {
		return err
	}
	return v.schema.Validate(value)
}

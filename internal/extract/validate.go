package extract

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type SchemaValidator struct {
	schema *jsonschema.Schema
}

func NewSchemaValidator(schemaStr string) (*SchemaValidator, error) {
	var doc any
	if err := json.Unmarshal([]byte(schemaStr), &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON Schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", doc); err != nil {
		return nil, fmt.Errorf("failed to add schema resource: %w", err)
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", err)
	}

	return &SchemaValidator{schema: schema}, nil
}

func (v *SchemaValidator) Validate(jsonStr string) error {
	var data any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if err := v.schema.Validate(data); err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			var msgs []string
			collectErrors(ve, &msgs)
			if len(msgs) == 1 {
				return fmt.Errorf("schema validation: %s", msgs[0])
			}
			return fmt.Errorf("schema validation failed:\n  - %s", strings.Join(msgs, "\n  - "))
		}
		return fmt.Errorf("schema validation failed: %v", err)
	}

	return nil
}

func collectErrors(ve *jsonschema.ValidationError, msgs *[]string) {
	if len(ve.Causes) == 0 {
		loc := strings.Join(ve.InstanceLocation, ".")
		if loc == "" {
			loc = "(root)"
		}
		*msgs = append(*msgs, fmt.Sprintf("%s: %v", loc, ve.ErrorKind))
		return
	}
	for _, cause := range ve.Causes {
		collectErrors(cause, msgs)
	}
}

package extract

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duffy1001/glean/llama"
)

type rootKind uint8

const (
	rootObject rootKind = iota
	rootArray
)

type preparedSchema struct {
	raw       string
	root      rootKind
	validator *schemaValidator
	grammar   string
}

var DefaultSchema = `{
  "type": "object",
  "properties": {
    "content_type": {"type": "string"},
    "summary": {"type": "string"},
    "attributes": {
      "type": "object",
      "additionalProperties": {
        "type": ["string", "number", "boolean", "null"]
      }
    },
    "warnings": {
      "type": "array",
      "items": {"type": "string"}
    }
  },
  "required": ["content_type", "summary", "attributes", "warnings"]
}`

func BuildSchemaFromFields(fields string) (string, error) {
	parts := strings.Split(fields, ",")
	props := make(map[string]interface{}, len(parts))
	seen := make(map[string]struct{}, len(parts))
	required := make([]string, 0, len(parts))
	for _, f := range parts {
		f = strings.TrimSpace(f)
		if f == "" {
			return "", fmt.Errorf("fields list cannot contain empty names")
		}
		if _, ok := seen[f]; ok {
			return "", fmt.Errorf("duplicate field %q", f)
		}
		seen[f] = struct{}{}
		props[f] = map[string]string{"type": "string"}
		required = append(required, f)
	}
	if len(required) == 0 {
		return "", fmt.Errorf("fields list cannot be empty")
	}
	itemSchema := map[string]interface{}{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
	schema := map[string]interface{}{
		"type":  "array",
		"items": itemSchema,
	}
	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func schemaHasRootType(schema, wanted string) bool {
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(schema), &doc); err != nil {
		return false
	}
	switch schemaType := doc["type"].(type) {
	case string:
		return schemaType == wanted
	case []interface{}:
		for _, value := range schemaType {
			if value == wanted {
				return true
			}
		}
	}
	return false
}

func jsonSchemaToGBNF(schemaStr string) (string, error) {
	gbnf, err := llama.SchemaToGrammar(schemaStr)
	if err != nil {
		return "", fmt.Errorf("schema conversion failed: %w", err)
	}
	return gbnf, nil
}

func prepareSchema(raw string, grammarEnabled bool) (*preparedSchema, error) {
	validator, err := newSchemaValidator(raw)
	if err != nil {
		return nil, err
	}
	prepared := &preparedSchema{raw: raw, validator: validator, root: rootObject}
	if schemaHasRootType(raw, "array") {
		prepared.root = rootArray
	}
	if grammarEnabled {
		grammar, err := jsonSchemaToGBNF(raw)
		if err != nil {
			return nil, err
		}
		prepared.grammar = grammar
	}
	return prepared, nil
}

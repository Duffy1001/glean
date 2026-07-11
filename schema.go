package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duffy/glean/llama"
)

var defaultSchema = `{
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

func buildSchemaFromFields(fields string) string {
	parts := strings.Split(fields, ",")
	props := make(map[string]interface{})
	required := make([]string, 0, len(parts))
	for _, f := range parts {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		props[f] = map[string]string{"type": "string"}
		required = append(required, f)
	}
	itemSchema := map[string]interface{}{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
	schema := map[string]interface{}{
		"type":  "array",
		"items": itemSchema,
	}
	b, _ := json.MarshalIndent(schema, "", "  ")
	return string(b)
}

func jsonSchemaToGBNF(schemaStr string) (string, error) {
	gbnf, err := llama.SchemaToGrammar(schemaStr)
	if err != nil {
		return "", fmt.Errorf("schema conversion failed: %w", err)
	}
	return gbnf, nil
}

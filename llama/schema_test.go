package llama

import (
	"strings"
	"testing"
)

func TestSchemaToGrammarObject(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number"}
		},
		"required": ["name", "age"]
	}`

	gbnf, err := SchemaToGrammar(schema)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}

	if !strings.Contains(gbnf, "root") {
		t.Errorf("grammar should contain root rule")
	}
	if !strings.Contains(gbnf, "name-kv") {
		t.Errorf("grammar should contain name-kv rule")
	}
	if !strings.Contains(gbnf, "age-kv") {
		t.Errorf("grammar should contain age-kv rule")
	}
}

func TestSchemaToGrammarArray(t *testing.T) {
	schema := `{
		"type": "array",
		"items": {"type": "string"}
	}`

	gbnf, err := SchemaToGrammar(schema)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}

	if !strings.Contains(gbnf, "root") {
		t.Errorf("grammar should contain root rule")
	}
}

func TestSchemaToGrammarEnum(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"status": {"type": "string", "enum": ["active", "inactive"]}
		},
		"required": ["status"]
	}`

	gbnf, err := SchemaToGrammar(schema)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}

	if !strings.Contains(gbnf, "active") {
		t.Errorf("grammar should contain enum value 'active'")
	}
	if !strings.Contains(gbnf, "inactive") {
		t.Errorf("grammar should contain enum value 'inactive'")
	}
}

func TestSchemaToGrammarInvalidJSON(t *testing.T) {
	_, err := SchemaToGrammar(`{invalid}`)
	if err == nil {
		t.Error("invalid JSON should produce error")
	}
}

func TestSchemaToGrammarUnsupportedPattern(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"val": {"type": "string", "pattern": "[a-z]+"}
		},
		"required": ["val"]
	}`

	_, err := SchemaToGrammar(schema)
	if err == nil {
		t.Error("pattern without ^...$ anchors should be rejected")
	}
}

func TestSchemaToGrammarNullable(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"val": {"type": ["string", "null"]}
		},
		"required": ["val"]
	}`

	gbnf, err := SchemaToGrammar(schema)
	if err != nil {
		t.Fatalf("nullable type should be supported: %v", err)
	}

	if !strings.Contains(gbnf, "null") {
		t.Errorf("grammar should handle null type")
	}
}

func TestSchemaToGrammarConst(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"version": {"const": "1.0"}
		},
		"required": ["version"]
	}`

	gbnf, err := SchemaToGrammar(schema)
	if err != nil {
		t.Fatalf("const should be supported: %v", err)
	}

	if !strings.Contains(gbnf, "1.0") {
		t.Errorf("grammar should contain const value")
	}
}

func TestSchemaToGrammarNestedObject(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"user": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"email": {"type": "string"}
				},
				"required": ["name", "email"]
			}
		},
		"required": ["user"]
	}`

	gbnf, err := SchemaToGrammar(schema)
	if err != nil {
		t.Fatalf("nested object should be supported: %v", err)
	}

	if !strings.Contains(gbnf, "name-kv") {
		t.Errorf("grammar should contain nested name field")
	}
	if !strings.Contains(gbnf, "email-kv") {
		t.Errorf("grammar should contain nested email field")
	}
}

func TestSchemaToGrammarEmpty(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {},
		"additionalProperties": {"type": "string"}
	}`

	_, err := SchemaToGrammar(schema)
	if err != nil {
		t.Fatalf("additionalProperties should be supported: %v", err)
	}
}

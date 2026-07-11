package main

import (
	"testing"
)

func TestBuildSchemaFromFields(t *testing.T) {
	schema := buildSchemaFromFields("name,age,email")

	v, err := NewSchemaValidator(schema)
	if err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	if err := v.Validate(`{"name":"Alice","age":"30","email":"alice@test.com"}`); err != nil {
		t.Errorf("valid data rejected: %v", err)
	}

	if err := v.Validate(`{"name":"Alice"}`); err == nil {
		t.Error("missing required fields should fail validation")
	}
}

func TestBuildSchemaFromFieldsEmpty(t *testing.T) {
	schema := buildSchemaFromFields("")
	v, err := NewSchemaValidator(schema)
	if err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if err := v.Validate(`{}`); err != nil {
		t.Errorf("empty object should be valid for empty schema: %v", err)
	}
}

func TestBuildSchemaFromFieldsWhitespace(t *testing.T) {
	schema := buildSchemaFromFields("  foo ,  bar  ,,")

	v, err := NewSchemaValidator(schema)
	if err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if err := v.Validate(`{"foo":"1","bar":"2"}`); err != nil {
		t.Errorf("whitespace handling failed: %v", err)
	}
}

func TestSchemaValidatorTypes(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"str": {"type": "string"},
			"num": {"type": "number"},
			"bool": {"type": "boolean"},
			"arr": {"type": "array", "items": {"type": "string"}}
		},
		"required": ["str", "num", "bool", "arr"]
	}`

	v, err := NewSchemaValidator(schema)
	if err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	if err := v.Validate(`{"str":"hello","num":42,"bool":true,"arr":["a","b"]}`); err != nil {
		t.Errorf("correct types rejected: %v", err)
	}

	if err := v.Validate(`{"str":42,"num":"x","bool":"yes","arr":[1,2]}`); err == nil {
		t.Error("wrong types should be rejected")
	}
}

func TestSchemaValidatorEnum(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"color": {"type": "string", "enum": ["red", "green", "blue"]}
		},
		"required": ["color"]
	}`

	v, err := NewSchemaValidator(schema)
	if err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	if err := v.Validate(`{"color":"red"}`); err != nil {
		t.Errorf("valid enum rejected: %v", err)
	}

	if err := v.Validate(`{"color":"purple"}`); err == nil {
		t.Error("invalid enum value should be rejected")
	}
}

func TestSchemaValidatorNullable(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"val": {"type": ["string", "null"]}
		},
		"required": ["val"]
	}`

	v, err := NewSchemaValidator(schema)
	if err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	if err := v.Validate(`{"val":"hello"}`); err != nil {
		t.Errorf("string value rejected: %v", err)
	}

	if err := v.Validate(`{"val":null}`); err != nil {
		t.Errorf("null value rejected: %v", err)
	}

	if err := v.Validate(`{"val":42}`); err == nil {
		t.Error("number should be rejected for string|null")
	}
}

func TestSchemaValidatorInvalidSchema(t *testing.T) {
	_, err := NewSchemaValidator(`{"type": invalid}`)
	if err == nil {
		t.Error("invalid JSON schema should fail")
	}
}

func TestSchemaValidatorAdditionalProperties(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"],
		"additionalProperties": {"type": "number"}
	}`

	v, err := NewSchemaValidator(schema)
	if err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	if err := v.Validate(`{"name":"test","score":100}`); err != nil {
		t.Errorf("valid additional property rejected: %v", err)
	}

	if err := v.Validate(`{"name":"test","score":"high"}`); err == nil {
		t.Error("wrong type additional property should be rejected")
	}
}

func TestSchemaValidatorArrayConstraints(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"tags": {
				"type": "array",
				"items": {"type": "string"},
				"minItems": 1,
				"maxItems": 3
			}
		},
		"required": ["tags"]
	}`

	v, err := NewSchemaValidator(schema)
	if err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	if err := v.Validate(`{"tags":["a","b"]}`); err != nil {
		t.Errorf("valid array rejected: %v", err)
	}

	if err := v.Validate(`{"tags":[]}`); err == nil {
		t.Error("empty array should violate minItems")
	}

	if err := v.Validate(`{"tags":["a","b","c","d"]}`); err == nil {
		t.Error("too many items should violate maxItems")
	}
}

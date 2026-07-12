//go:build integration

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEndExtraction(t *testing.T) {
	input := "John Doe, age 35, works at Acme Corp as a software engineer. Contact: john@example.com."

	cmd := exec.Command("./bin/glean", "--model", "fast", "--max-tokens", "200", "--fields", "name,age,employer,contact")
	cmd.Stdin = strings.NewReader(input)

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("glean failed: %v\nstderr: (check above)", err)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, string(out))
	}
	if len(result) == 0 {
		t.Fatalf("expected at least one extracted item: %s", string(out))
	}

	for _, key := range []string{"name", "age", "employer", "contact"} {
		if _, ok := result[0][key]; !ok {
			t.Errorf("missing key %q in output: %s", key, string(out))
		}
	}
}

func TestEndToEndDefaultSchema(t *testing.T) {
	input := "Server db-01 is running normally, 14 days uptime, no errors detected."

	cmd := exec.Command("./bin/glean", "--model", "fast", "--max-tokens", "200")
	cmd.Stdin = strings.NewReader(input)

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("glean failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, string(out))
	}

	for _, key := range []string{"content_type", "summary", "attributes", "warnings"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing key %q in output: %s", key, string(out))
		}
	}
}

func TestEndToEndCustomSchemaWithEnum(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"title": {"type": "string"},
			"category": {"type": "string", "enum": ["news", "blog", "docs"]}
		},
		"required": ["title", "category"]
	}`

	schemaPath := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(schemaPath, []byte(schema), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	cmd := exec.Command("./bin/glean", "--model", "fast", "--max-tokens", "200", "--schema", schemaPath)
	cmd.Stdin = strings.NewReader("This is a blog post about Go programming.")

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("glean failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, string(out))
	}

	category, ok := result["category"]
	if !ok {
		t.Fatalf("missing category in output")
	}
	validCats := map[string]bool{"news": true, "blog": true, "docs": true}
	if !validCats[category.(string)] {
		t.Errorf("category %q not in enum", category)
	}
}

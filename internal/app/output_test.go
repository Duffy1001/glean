package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteJSONCompact(t *testing.T) {
	var out bytes.Buffer
	if err := writeJSON(&out, json.RawMessage(`["one","two"]`), true); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != `["one","two"]` {
		t.Fatalf("unexpected compact output: %q", out.String())
	}
}

func TestWriteJSONIndented(t *testing.T) {
	var out bytes.Buffer
	if err := writeJSON(&out, json.RawMessage(`{"name":"Alice"}`), false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\n  \"name\": \"Alice\"\n") {
		t.Fatalf("unexpected indented output: %q", out.String())
	}
}

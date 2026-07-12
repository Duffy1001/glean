package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestJSONStreamArraySinkCompact(t *testing.T) {
	var buf bytes.Buffer
	sink := newJSONStreamArraySink(&buf, true)
	if err := sink.Start(); err != nil {
		t.Fatal(err)
	}
	if err := sink.WriteItem(map[string]any{"host": "worker-01", "status": "up"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.WriteItem(map[string]any{"host": "worker-01", "status": "down"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Finish(); err != nil {
		t.Fatal(err)
	}

	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	expected := []map[string]any{
		{"host": "worker-01", "status": "up"},
		{"host": "worker-01", "status": "down"},
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("unexpected output: %#v", got)
	}
	if strings.Contains(buf.String(), "\n  ") {
		t.Fatalf("compact output should not contain indented newlines: %q", buf.String())
	}
}

func TestJSONStreamArraySinkIndented(t *testing.T) {
	var buf bytes.Buffer
	sink := newJSONStreamArraySink(&buf, false)
	if err := sink.Start(); err != nil {
		t.Fatal(err)
	}
	if err := sink.WriteItem(map[string]any{"host": "worker-01", "status": "up"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.WriteItem(map[string]any{"host": "worker-01", "status": "down"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Finish(); err != nil {
		t.Fatal(err)
	}

	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	expected := []map[string]any{
		{"host": "worker-01", "status": "up"},
		{"host": "worker-01", "status": "down"},
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("unexpected output: %#v", got)
	}
	if !strings.Contains(buf.String(), "\n  {") {
		t.Fatalf("indented output should include array-aligned indentation: %q", buf.String())
	}
}

func TestJSONStreamArraySinkEmpty(t *testing.T) {
	for _, compact := range []bool{true, false} {
		var buf bytes.Buffer
		sink := newJSONStreamArraySink(&buf, compact)
		if err := sink.Start(); err != nil {
			t.Fatal(err)
		}
		if err := sink.Finish(); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(buf.String()) != "[]" {
			t.Fatalf("compact=%v expected [] got %q", compact, buf.String())
		}
	}
}

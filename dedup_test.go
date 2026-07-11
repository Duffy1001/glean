package main

import (
	"encoding/json"
	"testing"
)

func mustParse(t *testing.T, s string) []interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	arr, ok := v.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T", v)
	}
	return arr
}

func TestDedupNoDuplicates(t *testing.T) {
	in := mustParse(t, `[{"name":"a"},{"name":"b"},{"name":"c"}]`)
	out := dedupByPK(in, "name")
	if len(out) != 3 {
		t.Errorf("expected 3 records, got %d", len(out))
	}
}

func TestDedupExactDuplicates(t *testing.T) {
	in := mustParse(t, `[{"name":"a","v":"1"},{"name":"a","v":"1"},{"name":"b","v":"2"}]`)
	out := dedupByPK(in, "name")
	if len(out) != 2 {
		t.Errorf("expected 2 records, got %d", len(out))
	}
}

func TestDedupMergesFields(t *testing.T) {
	in := mustParse(t, `[
		{"name":"db01","size":"500G"},
		{"name":"db01","type":"disk"},
		{"name":"db02","size":"1T"}
	]`)
	out := dedupByPK(in, "name")
	if len(out) != 2 {
		t.Fatalf("expected 2 records, got %d", len(out))
	}
	merged := out[0].(map[string]interface{})
	if merged["size"] != "500G" {
		t.Errorf("expected size 500G, got %v", merged["size"])
	}
	if merged["type"] != "disk" {
		t.Errorf("expected type disk, got %v", merged["type"])
	}
}

func TestDedupLastValueWins(t *testing.T) {
	in := mustParse(t, `[{"name":"a","status":"up"},{"name":"a","status":"down"}]`)
	out := dedupByPK(in, "name")
	if len(out) != 1 {
		t.Fatalf("expected 1 record, got %d", len(out))
	}
	rec := out[0].(map[string]interface{})
	if rec["status"] != "down" {
		t.Errorf("expected last value 'down', got %v", rec["status"])
	}
}

func TestDedupPreservesOrder(t *testing.T) {
	in := mustParse(t, `[{"name":"c"},{"name":"a"},{"name":"b"},{"name":"a"}]`)
	out := dedupByPK(in, "name")
	if len(out) != 3 {
		t.Fatalf("expected 3 records, got %d", len(out))
	}
	first := out[0].(map[string]interface{})["name"]
	if first != "c" {
		t.Errorf("expected first record 'c', got %v", first)
	}
}

func TestDedupMissingPK(t *testing.T) {
	in := mustParse(t, `[{"name":"a"},{"other":"b"},{"name":"c"}]`)
	out := dedupByPK(in, "name")
	if len(out) != 3 {
		t.Errorf("records without pk should pass through, got %d", len(out))
	}
}

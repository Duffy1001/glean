package extract

import (
	"strings"
	"testing"
)

func TestStreamReaderChunksPreservesRecords(t *testing.T) {
	input := "one\ntwo\nthree\nfour\nfive\nsix\nseven\n"
	var chunks []string
	hadInput, err := StreamReaderChunks(strings.NewReader(input), 12, "\n", func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hadInput {
		t.Fatal("expected input")
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, line := range strings.Split(strings.TrimSpace(input), "\n") {
		found := false
		for _, chunk := range chunks {
			if strings.Contains(chunk, line) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("line %q was lost", line)
		}
	}
}

func TestStreamReaderChunksEmptyInput(t *testing.T) {
	hadInput, err := StreamReaderChunks(strings.NewReader(" \n\t"), 10, "\n", func(string) error {
		t.Fatal("callback should not be called")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if hadInput {
		t.Fatal("whitespace-only input should be empty")
	}
}

func TestStreamReaderChunksRespectsByteLimit(t *testing.T) {
	var chunks []string
	_, err := StreamReaderChunks(strings.NewReader("one\ntwo\nthree\nfour\nfive\n"), 8, "\n", func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0] != "one\ntwo" || chunks[1] != "three" || chunks[2] != "four\nfive" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestSplitChunk(t *testing.T) {
	left, right, ok := SplitChunk("one||two||three||four", "||")
	if !ok {
		t.Fatal("expected split")
	}
	if left+"||"+right != "one||two||three||four" {
		t.Fatalf("split changed input: %q + %q", left, right)
	}
}

func TestSplitChunkFallsBackToRunes(t *testing.T) {
	left, right, ok := SplitChunk("abcdef", "||")
	if !ok {
		t.Fatal("expected split")
	}
	if left+right != "abcdef" {
		t.Fatalf("split changed input: %q + %q", left, right)
	}
}

func TestSchemaHasRootType(t *testing.T) {
	if !SchemaHasRootType(`{"type":"array"}`, "array") {
		t.Fatal("array schema not detected")
	}
	if SchemaHasRootType(`{"type":"object"}`, "array") {
		t.Fatal("object schema detected as array")
	}
}

func TestSchemaHasRootTypeVariants(t *testing.T) {
	tests := []struct {
		schema string
		wanted string
		want   bool
	}{
		{`{"type":"array"}`, "array", true},
		{`{"type":"object"}`, "array", false},
		{`{"type":["array","null"]}`, "array", true},
		{`{"type":["object","null"]}`, "array", false},
		{`{"type":["string","array"]}`, "array", true},
		{`{"properties":{}}`, "array", false},
		{`{"type":42}`, "array", false},
	}
	for _, tc := range tests {
		got := SchemaHasRootType(tc.schema, tc.wanted)
		if got != tc.want {
			t.Errorf("SchemaHasRootType(%q, %q) = %v, want %v", tc.schema, tc.wanted, got, tc.want)
		}
	}
}

func TestDecodeDelimiter(t *testing.T) {
	got, err := DecodeDelimiter(`\n`)
	if err != nil || got != "\n" {
		t.Fatalf("newline delimiter: %q, %v", got, err)
	}
	got, err = DecodeDelimiter(`\r`)
	if err != nil || got != "\r" {
		t.Fatalf("carriage-return delimiter: %q, %v", got, err)
	}
	got, err = DecodeDelimiter(`\0`)
	if err != nil || got != "\x00" {
		t.Fatalf("NUL delimiter: %q, %v", got, err)
	}
	got, err = DecodeDelimiter(`\\`)
	if err != nil || got != "\\" {
		t.Fatalf("backslash delimiter: %q, %v", got, err)
	}
	if _, err := DecodeDelimiter(``); err == nil {
		t.Fatal("empty delimiter should fail")
	}
}

func TestDecodeDelimiterMultiCharacter(t *testing.T) {
	got, err := DecodeDelimiter(`||`)
	if err != nil || got != "||" {
		t.Fatalf("multi-char delimiter: %q, %v", got, err)
	}
	got, err = DecodeDelimiter(`\n\t`)
	if err != nil || got != "\n\t" {
		t.Fatalf("combined escapes: %q, %v", got, err)
	}
	got, err = DecodeDelimiter(`--`)
	if err != nil || got != "--" {
		t.Fatalf("literal dashes: %q, %v", got, err)
	}
}

func TestDecodeDelimiterTrailingEscape(t *testing.T) {
	if _, err := DecodeDelimiter(`\`); err == nil {
		t.Fatal("trailing escape should fail")
	}
}

func TestStreamReaderChunksUsesDelimiter(t *testing.T) {
	var chunks []string
	_, err := StreamReaderChunks(strings.NewReader("one||two||three"), 7, "||", func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0] != "one||two" || chunks[1] != "three" {
		t.Fatalf("unexpected delimiter chunks: %#v", chunks)
	}
}

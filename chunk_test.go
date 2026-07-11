package main

import (
	"strings"
	"testing"
)

func TestStreamReaderChunksPreservesInputWithOverlap(t *testing.T) {
	input := "one\ntwo\nthree\nfour\nfive\nsix\nseven\n"
	var chunks []string
	hadInput, err := streamReaderChunks(strings.NewReader(input), 12, func(chunk string) error {
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
	hadInput, err := streamReaderChunks(strings.NewReader(" \n\t"), 10, func(string) error {
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

func TestSplitChunk(t *testing.T) {
	left, right, ok := splitChunk("one\ntwo\nthree\nfour")
	if !ok {
		t.Fatal("expected split")
	}
	if left+"\n"+right != "one\ntwo\nthree\nfour" {
		t.Fatalf("split changed input: %q + %q", left, right)
	}
}

func TestSchemaHasRootType(t *testing.T) {
	if !schemaHasRootType(`{"type":"array"}`, "array") {
		t.Fatal("array schema not detected")
	}
	if schemaHasRootType(`{"type":"object"}`, "array") {
		t.Fatal("object schema detected as array")
	}
}

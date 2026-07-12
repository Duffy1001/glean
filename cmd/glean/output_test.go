package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestEmitBufferedArray(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	if err := emitBufferedArray([]interface{}{"one", "two"}, true); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != `["one","two"]` {
		t.Fatalf("unexpected atomic output: %q", string(out))
	}
}

func TestEmitBufferedArrayEmpty(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	if err := emitBufferedArray(nil, true); err != nil {
		t.Fatal(err)
	}
	w.Close()
	out, _ := io.ReadAll(r)
	if strings.TrimSpace(string(out)) != "[]" {
		t.Fatalf("empty array output: %q", string(out))
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSourcesPreservesOrder(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.txt")
	p2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(p1, []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readSources([]string{p1, p2})
	if err != nil {
		t.Fatal(err)
	}
	idxA := strings.Index(got, "alpha")
	idxB := strings.Index(got, "beta")
	if idxA < 0 || idxB < 0 {
		t.Fatalf("missing content: %q", got)
	}
	if idxA > idxB {
		t.Fatalf("file order not preserved: alpha at %d, beta at %d", idxA, idxB)
	}
}

func TestReadSourcesStdinFallback(t *testing.T) {
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		w.Write([]byte("from-stdin"))
		w.Close()
	}()

	got, err := readSources(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "from-stdin") {
		t.Fatalf("stdin content missing: %q", got)
	}
}

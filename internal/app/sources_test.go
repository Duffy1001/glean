package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSourcesUsesInjectedStdin(t *testing.T) {
	sources, closeSources, err := openSources(nil, strings.NewReader("stdin"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeSources()
	if len(sources) != 1 || sources[0].Name != "stdin" {
		t.Fatalf("unexpected sources: %#v", sources)
	}
	data, err := io.ReadAll(sources[0].Reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "stdin" {
		t.Fatalf("unexpected stdin data: %q", data)
	}
}

func TestOpenSourcesPreservesOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}

	sources, closeSources, err := openSources([]string{first, second}, strings.NewReader("unused"))
	if err != nil {
		t.Fatal(err)
	}
	if err := closeSources(); err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].Name != first || sources[1].Name != second {
		t.Fatalf("unexpected source order: %#v", sources)
	}
}

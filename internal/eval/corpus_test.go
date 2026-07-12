package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCorpusPreservesManifestOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cases", "second"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cases", "first"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCorpusFile(t, filepath.Join(root, "manifest.json"), `{"version":1,"model":"fast","cases":["second","first"]}`)
	writeCase(t, root, "first", `{"id":"first","description":"first case","tier":"conformance","delimiter":"\n","comparison":"positional"}`)
	writeCase(t, root, "second", `{"id":"second","description":"second case","tier":"capability","delimiter":"\n","comparison":"ordered"}`)

	corpus, err := LoadCorpus(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 2 || corpus.Cases[0].Case.ID != "second" || corpus.Cases[1].Case.ID != "first" {
		t.Fatalf("manifest order not preserved: %#v", corpus.Cases)
	}
}

func TestLoadCorpusRejectsExpectedOutputOutsideSchema(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cases", "bad"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCorpusFile(t, filepath.Join(root, "manifest.json"), `{"version":1,"model":"fast","cases":["bad"]}`)
	writeCorpusFile(t, filepath.Join(root, "cases", "bad", "case.json"), `{"id":"bad","description":"bad case","tier":"conformance","delimiter":"\n","comparison":"positional"}`)
	writeCorpusFile(t, filepath.Join(root, "cases", "bad", "input.txt"), "input")
	writeCorpusFile(t, filepath.Join(root, "cases", "bad", "schema.json"), `{"type":"object","properties":{"age":{"type":"integer"}},"required":["age"]}`)
	writeCorpusFile(t, filepath.Join(root, "cases", "bad", "expected.json"), `{"age":"30"}`)
	if _, err := LoadCorpus(root); err == nil {
		t.Fatal("expected invalid fixture to fail")
	}
}

func writeCase(t *testing.T, root, id, metadata string) {
	t.Helper()
	dir := filepath.Join(root, "cases", id)
	writeCorpusFile(t, filepath.Join(dir, "case.json"), metadata)
	writeCorpusFile(t, filepath.Join(dir, "input.txt"), "input")
	writeCorpusFile(t, filepath.Join(dir, "schema.json"), `{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`)
	writeCorpusFile(t, filepath.Join(dir, "expected.json"), `{"value":"expected"}`)
}

func writeCorpusFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

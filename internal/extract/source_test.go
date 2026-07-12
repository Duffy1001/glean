package extract

import (
	"strings"
	"testing"
)

func TestReadSourcesPreservesOrder(t *testing.T) {
	sources := []Source{
		{Name: "a", Reader: strings.NewReader("alpha")},
		{Name: "b", Reader: strings.NewReader("beta")},
	}

	got, err := ReadSources(sources)
	if err != nil {
		t.Fatal(err)
	}
	idxA := strings.Index(got, "alpha")
	idxB := strings.Index(got, "beta")
	if idxA < 0 || idxB < 0 {
		t.Fatalf("missing content: %q", got)
	}
	if idxA > idxB {
		t.Fatalf("source order not preserved: alpha at %d, beta at %d", idxA, idxB)
	}
}

func TestReadSourcesEmpty(t *testing.T) {
	got, err := ReadSources(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestStreamSourcesFileBoundaries(t *testing.T) {
	sources := []Source{
		{Name: "a", Reader: strings.NewReader("one\ntwo")},
		{Name: "b", Reader: strings.NewReader("three\nfour")},
	}

	var chunks []string
	hadInput, err := StreamSources(sources, 100, "\n", func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hadInput {
		t.Fatal("expected input")
	}

	allContent := strings.Join(chunks, "\n")
	for _, expected := range []string{"one", "two", "three", "four"} {
		if !strings.Contains(allContent, expected) {
			t.Errorf("record %q was lost", expected)
		}
	}
}

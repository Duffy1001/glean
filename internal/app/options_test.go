package app

import "testing"

func TestParseOptionsTracksExplicitFields(t *testing.T) {
	opts, err := ParseOptions([]string{"--fields", "", "input.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.FieldsProvided {
		t.Fatal("expected explicit --fields to be tracked")
	}
	if opts.Fields != "" {
		t.Fatalf("fields = %q, want empty", opts.Fields)
	}
	if len(opts.InputPaths) != 1 || opts.InputPaths[0] != "input.txt" {
		t.Fatalf("unexpected input paths: %#v", opts.InputPaths)
	}
}

func TestParseOptionsDefaults(t *testing.T) {
	opts, err := ParseOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.FieldsProvided {
		t.Fatal("fields should not be marked as provided by default")
	}
	if opts.Model != "fast" || opts.MaxTokens != 2048 || opts.Context != 8192 {
		t.Fatalf("unexpected defaults: %#v", opts)
	}
}

func TestParseOptionsRejectsUnknownFlag(t *testing.T) {
	if _, err := ParseOptions([]string{"--unknown"}); err == nil {
		t.Fatal("unknown flag should fail")
	}
}

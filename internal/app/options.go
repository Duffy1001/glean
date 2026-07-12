package app

import (
	"flag"
	"fmt"
	"io"

	"github.com/duffy1001/glean"
)

type Options struct {
	SchemaFile     string
	Fields         string
	FieldsProvided bool
	Model          string
	MaxTokens      int
	Compact        bool
	Atomic         bool
	Threads        int
	Context        int
	Delimiter      string
	NoGrammar      bool
	Verbose        bool
	Device         string
	GPULayers      int
	ShowVersion    bool
	ShowReport     bool
	InputPaths     []string
}

func ParseOptions(args []string) (Options, error) {
	var opts Options
	fs := flag.NewFlagSet("glean", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.SchemaFile, "schema", "", "JSON Schema file for constrained output")
	fs.StringVar(&opts.Fields, "fields", "", "Comma-separated field names for simple schema; names must be unique and non-empty")
	fs.StringVar(&opts.Model, "model", glean.DefaultModel(), "Model: fast (0.6B)")
	fs.IntVar(&opts.MaxTokens, "max-tokens", 2048, "Maximum tokens to generate")
	fs.BoolVar(&opts.Compact, "compact", false, "Output compact JSON")
	fs.BoolVar(&opts.Atomic, "atomic", false, "Buffer array output and emit only after all chunks succeed")
	fs.IntVar(&opts.Threads, "threads", 4, "CPU threads")
	fs.IntVar(&opts.Context, "ctx", 8192, "Context window size")
	fs.StringVar(&opts.Delimiter, "delimiter", "\\n", "Record delimiter for array extraction (supports \\n, \\t, \\r, \\0, \\\\, and multi-character strings)")
	fs.BoolVar(&opts.NoGrammar, "no-grammar", false, "Disable grammar-constrained generation")
	fs.BoolVar(&opts.Verbose, "verbose", false, "Show llama.cpp debug output")
	fs.StringVar(&opts.Device, "device", "auto", "Inference device: auto, cpu, or gpu")
	fs.IntVar(&opts.GPULayers, "gpu-layers", -1, "Model layers to offload (-1 means all available)")
	fs.BoolVar(&opts.ShowVersion, "version", false, "Show version and build edition")
	fs.BoolVar(&opts.ShowReport, "report", false, "Report available inference backends and devices as JSON")
	if err := fs.Parse(args); err != nil {
		return Options{}, fmt.Errorf("parse options: %w", err)
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "fields" {
			opts.FieldsProvided = true
		}
	})
	opts.InputPaths = fs.Args()
	return opts, nil
}

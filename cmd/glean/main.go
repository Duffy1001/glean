package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/duffy1001/glean"
	"github.com/duffy1001/glean/internal/extract"
	"github.com/duffy1001/glean/llama"
)

func main() {
	schemaFile := flag.String("schema", "", "JSON Schema file for constrained output")
	fields := flag.String("fields", "", "Comma-separated field names for simple schema; names must be unique and non-empty")
	modelChoice := flag.String("model", glean.DefaultModel(), "Model: fast (0.6B)")
	maxTokens := flag.Int("max-tokens", 2048, "Maximum tokens to generate")
	compact := flag.Bool("compact", false, "Output compact JSON")
	atomic := flag.Bool("atomic", false, "Buffer array output and emit only after all chunks succeed")
	nThreads := flag.Int("threads", 4, "CPU threads")
	nCtx := flag.Int("ctx", 8192, "Context window size")
	delimiter := flag.String("delimiter", "\\n", "Record delimiter for array extraction (supports \\n, \\t, \\r, \\0, \\\\, and multi-character strings)")
	noGrammar := flag.Bool("no-grammar", false, "Disable grammar-constrained generation")
	verbose := flag.Bool("verbose", false, "Show llama.cpp debug output")
	device := flag.String("device", "auto", "Inference device: auto, cpu, or gpu")
	gpuLayers := flag.Int("gpu-layers", -1, "Model layers to offload (-1 means all available)")
	showVersion := flag.Bool("version", false, "Show version and build edition")
	showReport := flag.Bool("report", false, "Report available inference backends and devices as JSON")
	flag.Parse()
	var err error
	if *showVersion {
		fmt.Printf("glean %s (%s)\n", glean.Version, glean.BuildVariant())
		return
	}

	fieldsProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "fields" {
			fieldsProvided = true
		}
	})

	if *verbose {
		llama.SetLogLevel(1)
	} else {
		llama.SetLogLevel(6)
	}
	verbosef := func(format string, args ...interface{}) {
		if *verbose {
			fmt.Fprintf(os.Stderr, format, args...)
		}
	}
	if err := extract.InitializeBackend(); err != nil {
		fmt.Fprintf(os.Stderr, "Backend error: %v\n", err)
		os.Exit(1)
	}
	defer extract.ShutdownBackend()

	devices := llama.BackendDevices()
	hasGPU := false
	for _, backendDevice := range devices {
		if backendDevice.Type == "gpu" || backendDevice.Type == "igpu" {
			hasGPU = true
			break
		}
	}
	if *showReport {
		expectedAccelerator := "vulkan"
		defaultDevice := "cpu"
		if hasGPU {
			defaultDevice = "gpu"
		}
		if runtime.GOOS == "darwin" {
			expectedAccelerator = "metal"
		}
		report := struct {
			Version             string                `json:"version"`
			Variant             string                `json:"variant"`
			OS                  string                `json:"os"`
			Architecture        string                `json:"architecture"`
			ExpectedAccelerator string                `json:"expected_accelerator"`
			AccelerationReady   bool                  `json:"acceleration_ready"`
			DefaultDevice       string                `json:"default_device"`
			Backends            []string              `json:"backends"`
			Devices             []llama.BackendDevice `json:"devices"`
		}{glean.Version, glean.BuildVariant(), runtime.GOOS, runtime.GOARCH, expectedAccelerator, hasGPU, defaultDevice, llama.Backends(), devices}
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Report error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(out))
		return
	}

	switch *device {
	case "auto":
		if !hasGPU {
			*gpuLayers = 0
		}
	case "cpu":
		*gpuLayers = 0
	case "gpu":
		if !hasGPU {
			fmt.Fprintln(os.Stderr, "No usable GPU backend detected; use --report for details")
			os.Exit(1)
		}
		if *gpuLayers == 0 {
			fmt.Fprintln(os.Stderr, "--device gpu conflicts with --gpu-layers 0")
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown device %q (available: auto, cpu, gpu)\n", *device)
		os.Exit(1)
	}

	args := flag.Args()

	schema := extract.DefaultSchema
	if *schemaFile != "" {
		data, err := os.ReadFile(*schemaFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading schema: %v\n", err)
			os.Exit(1)
		}
		schema = string(data)
	} else if fieldsProvided {
		schema, err = extract.BuildSchemaFromFields(*fields)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Field schema error: %v\n", err)
			os.Exit(1)
		}
	}

	modelPath, err := glean.ResolveModel(*modelChoice, *verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	verbosef("Loading model (%s)...\n", *modelChoice)
	start := time.Now()
	engine, err := extract.NewEngine(context.Background(), extract.Config{
		ModelPath:        modelPath,
		ContextSize:      *nCtx,
		Threads:          *nThreads,
		GPULayers:        *gpuLayers,
		Device:           *device,
		AllowCPUFallback: *device == "auto",
		GrammarEnabled:   !*noGrammar,
		Verbose:          *verbose,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading model: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()
	verbosef("Model loaded in %v\n", time.Since(start))

	delim, err := extract.DecodeDelimiter(*delimiter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Delimiter error: %v\n", err)
		os.Exit(1)
	}
	var sources []extract.Source
	if len(args) == 0 {
		sources = []extract.Source{{Name: "stdin", Reader: os.Stdin}}
	} else {
		for _, p := range args {
			f, err := os.Open(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()
			sources = append(sources, extract.Source{Name: p, Reader: f})
		}
	}

	result, err := engine.Extract(context.Background(), extract.Request{
		Schema:    schema,
		MaxTokens: *maxTokens,
		Delimiter: delim,
	}, sources)
	if err != nil {
		if errors.Is(err, extract.ErrNoInput) {
			fmt.Fprintln(os.Stderr, "No input provided")
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}

	// Extract always returns a complete document. Streaming sinks restore the
	// default incremental array behavior in the later output-policy refactor.
	_ = atomic

	var parsed interface{}
	if err := json.Unmarshal(result.JSON, &parsed); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid JSON output: %v\n", err)
		os.Exit(1)
	}
	var out []byte
	if *compact {
		out, err = json.Marshal(parsed)
	} else {
		out, err = json.MarshalIndent(parsed, "", "  ")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))

	if result.GeneratedTokens > 0 {
		verbosef("Generated %d tokens in %v (%.1f tok/s)\n",
			result.GeneratedTokens, result.TotalTime, float64(result.GeneratedTokens)/result.TotalTime.Seconds())
	}

}

func emitBufferedArray(items []interface{}, compact bool) error {
	if len(items) == 0 {
		fmt.Println("[]")
		return nil
	}

	fmt.Print("[")
	for i, item := range items {
		if i > 0 {
			fmt.Print(",")
		}
		var data []byte
		var err error
		if compact {
			data, err = json.Marshal(item)
		} else {
			data, err = json.MarshalIndent(item, "", "  ")
		}
		if err != nil {
			return err
		}
		fmt.Print(string(data))
	}
	fmt.Println("]")
	return nil
}

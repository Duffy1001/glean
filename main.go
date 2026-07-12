package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/duffy1001/glean/internal/extract"
	"github.com/duffy1001/glean/llama"
)

const (
	promptOverheadTokens = 600
	charsPerInputToken   = 3.0
)

func main() {
	schemaFile := flag.String("schema", "", "JSON Schema file for constrained output")
	fields := flag.String("fields", "", "Comma-separated field names for simple schema; names must be unique and non-empty")
	modelChoice := flag.String("model", defaultModel(), "Model: fast (0.6B)")
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
		fmt.Printf("glean %s (%s)\n", version, buildVariant())
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
	llama.BackendInit()
	defer llama.BackendFree()

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
		}{version, buildVariant(), runtime.GOOS, runtime.GOARCH, expectedAccelerator, hasGPU, defaultDevice, llama.Backends(), devices}
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

	validator, err := extract.NewSchemaValidator(schema)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Schema error: %v\n", err)
		os.Exit(1)
	}

	modelPath, err := resolveModel(*modelChoice, *verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	verbosef("Loading model (%s)...\n", *modelChoice)
	start := time.Now()
	m, err := llama.Load(modelPath, *nCtx, *nThreads, *gpuLayers, *device == "auto")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading model: %v\n", err)
		os.Exit(1)
	}
	defer m.Free()
	verbosef("Model loaded in %v\n", time.Since(start))

	var grammar string
	if !*noGrammar {
		grammar, err = extract.JSONSchemaToGBNF(schema)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Schema conversion error: %v\n", err)
			os.Exit(1)
		}
		if err := m.SetGrammar(grammar, "root"); err != nil {
			fmt.Fprintf(os.Stderr, "Grammar error: %v\n", err)
			os.Exit(1)
		}
	}

	eos := m.TokenEOS()

	delim, err := extract.DecodeDelimiter(*delimiter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Delimiter error: %v\n", err)
		os.Exit(1)
	}
	inputBudget := *nCtx - promptOverheadTokens - *maxTokens
	if inputBudget < 1 {
		fmt.Fprintln(os.Stderr, "Context is too small for the requested max-tokens")
		os.Exit(1)
	}
	inputCharsBudget := int(float64(inputBudget) * charsPerInputToken)

	totalGenerated := 0
	genStart := time.Now()
	needsReset := false
	chunkNumber := 0

	reset := func() error {
		if !needsReset {
			needsReset = true
			return nil
		}
		if err := m.ClearContext(); err != nil {
			return err
		}
		if grammar != "" {
			if err := m.SetGrammar(grammar, "root"); err != nil {
				return fmt.Errorf("reset grammar: %w", err)
			}
		}
		return nil
	}

	var processArrayChunk func(string) error
	arrayStarted := false
	arrayItems := 0
	bufferedArrayItems := make([]interface{}, 0)
	writeArrayItem := func(item interface{}) error {
		if *atomic {
			bufferedArrayItems = append(bufferedArrayItems, item)
			return nil
		}
		if !arrayStarted {
			fmt.Print("[")
			arrayStarted = true
		}
		if arrayItems > 0 {
			fmt.Print(",")
		}
		var data []byte
		var marshalErr error
		if *compact {
			data, marshalErr = json.Marshal(item)
		} else {
			data, marshalErr = json.MarshalIndent(item, "", "  ")
		}
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Print(string(data))
		arrayItems++
		return nil
	}

	processArrayChunk = func(chunk string) error {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			return nil
		}
		if err := reset(); err != nil {
			return err
		}
		chunkNumber++
		verbosef("Processing chunk %d (%d bytes)...\n", chunkNumber, len(chunk))
		raw, generated, hitLimit, err := extract.GenerateOne(m, schema, chunk, *maxTokens, *nCtx, eos, *verbose)
		totalGenerated += generated
		if err != nil {
			if errors.Is(err, extract.ErrPromptTooLong) {
				left, right, ok := extract.SplitChunk(chunk, delim)
				if ok {
					if err := processArrayChunk(left); err != nil {
						return err
					}
					return processArrayChunk(right)
				}
			}
			return fmt.Errorf("chunk %d: %w", chunkNumber, err)
		}

		var parsed []interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			if hitLimit {
				left, right, ok := extract.SplitChunk(chunk, delim)
				if ok {
					if err := processArrayChunk(left); err != nil {
						return err
					}
					return processArrayChunk(right)
				}
			}
			return fmt.Errorf("chunk %d produced invalid JSON: %w", chunkNumber, err)
		}
		if err := validator.Validate(raw); err != nil {
			return fmt.Errorf("chunk %d validation: %w", chunkNumber, err)
		}
		for _, item := range parsed {
			if err := writeArrayItem(item); err != nil {
				return fmt.Errorf("chunk %d output: %w", chunkNumber, err)
			}
		}
		return nil
	}

	arraySchema := extract.SchemaHasRootType(schema, "array")
	if arraySchema {
		hadInput, err := extract.StreamSources(sources, inputCharsBudget, delim, processArrayChunk)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !hadInput {
			fmt.Fprintln(os.Stderr, "No input provided")
			os.Exit(1)
		}
		if *atomic {
			if err := emitBufferedArray(bufferedArrayItems, *compact); err != nil {
				fmt.Fprintf(os.Stderr, "Atomic output error: %v\n", err)
				os.Exit(1)
			}
		} else if !arrayStarted {
			fmt.Print("[]")
		} else {
			fmt.Print("]")
		}
		fmt.Println()
	} else {
		input, err := extract.ReadSources(sources)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			fmt.Fprintln(os.Stderr, "No input provided")
			os.Exit(1)
		}
		if err := reset(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		raw, generated, _, err := extract.GenerateOne(m, schema, input, *maxTokens, *nCtx, eos, *verbose)
		totalGenerated += generated
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := validator.Validate(raw); err != nil {
			fmt.Fprintf(os.Stderr, "Validation error: %v\n", err)
			os.Exit(1)
		}
		var parsed interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
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
	}

	if totalGenerated > 0 {
		verbosef("Generated %d tokens in %v (%.1f tok/s)\n",
			totalGenerated, time.Since(genStart), float64(totalGenerated)/time.Since(genStart).Seconds())
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

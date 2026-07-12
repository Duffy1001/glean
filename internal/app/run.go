package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/duffy1001/glean"
	"github.com/duffy1001/glean/internal/extract"
	"github.com/duffy1001/glean/llama"
)

func Run(ctx context.Context, opts Options, stdin io.Reader, stdout, stderr io.Writer) (retErr error) {
	if opts.ShowVersion {
		_, err := fmt.Fprintf(stdout, "glean %s (%s)\n", glean.Version, glean.BuildVariant())
		return err
	}

	if opts.Verbose {
		llama.SetLogLevel(1)
	} else {
		llama.SetLogLevel(6)
	}
	verbosef := func(format string, args ...interface{}) {
		if opts.Verbose {
			fmt.Fprintf(stderr, format, args...)
		}
	}

	if err := extract.InitializeBackend(); err != nil {
		return fmt.Errorf("backend: %w", err)
	}
	devices := llama.BackendDevices()
	hasGPU := false
	for _, backendDevice := range devices {
		if backendDevice.Type == "gpu" || backendDevice.Type == "igpu" {
			hasGPU = true
			break
		}
	}
	if opts.ShowReport {
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
			return fmt.Errorf("marshal report: %w", err)
		}
		_, err = fmt.Fprintln(stdout, string(out))
		return err
	}

	switch opts.Device {
	case "auto":
		if !hasGPU {
			opts.GPULayers = 0
		}
	case "cpu":
		opts.GPULayers = 0
	case "gpu":
		if !hasGPU {
			return errors.New("no usable GPU backend detected; use --report for details")
		}
		if opts.GPULayers == 0 {
			return errors.New("--device gpu conflicts with --gpu-layers 0")
		}
	default:
		return fmt.Errorf("unknown device %q (available: auto, cpu, gpu)", opts.Device)
	}

	schema := extract.DefaultSchema
	switch {
	case opts.SchemaFile != "":
		data, err := os.ReadFile(opts.SchemaFile)
		if err != nil {
			return fmt.Errorf("error reading schema: %w", err)
		}
		schema = string(data)
	case opts.FieldsProvided:
		built, err := extract.BuildSchemaFromFields(opts.Fields)
		if err != nil {
			return fmt.Errorf("field schema error: %w", err)
		}
		schema = built
	}

	delimiter, err := extract.DecodeDelimiter(opts.Delimiter)
	if err != nil {
		return fmt.Errorf("delimiter error: %w", err)
	}
	modelPath, err := glean.ResolveModel(opts.Model, opts.Verbose)
	if err != nil {
		return err
	}

	verbosef("Loading model (%s)...\n", opts.Model)
	loadStart := time.Now()
	engine, err := extract.NewEngine(ctx, extract.Config{
		ModelPath:        modelPath,
		ContextSize:      opts.Context,
		Threads:          opts.Threads,
		GPULayers:        opts.GPULayers,
		Device:           opts.Device,
		AllowCPUFallback: opts.Device == "auto",
		GrammarEnabled:   !opts.NoGrammar,
		Verbose:          opts.Verbose,
	})
	if err != nil {
		return fmt.Errorf("error loading model: %w", err)
	}
	defer engine.Close()
	verbosef("Model loaded in %v\n", time.Since(loadStart))

	sources, closeSources, err := openSources(opts.InputPaths, stdin)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, closeSources())
	}()

	result, err := engine.Extract(ctx, extract.Request{
		Schema:    schema,
		MaxTokens: opts.MaxTokens,
		Delimiter: delimiter,
	}, sources)
	if err != nil {
		if errors.Is(err, extract.ErrNoInput) {
			return errors.New("No input provided")
		}
		return err
	}

	// Extract returns a complete document, so it already satisfies --atomic.
	// Streaming sinks restore incremental array output in the later sink refactor.
	_ = opts.Atomic
	if err := writeJSON(stdout, result.JSON, opts.Compact); err != nil {
		return err
	}
	if result.Metrics.GeneratedTokens > 0 {
		verbosef("Generated %d tokens in %v (%.1f tok/s)\n",
			result.Metrics.GeneratedTokens, result.Metrics.TotalTime, float64(result.Metrics.GeneratedTokens)/result.Metrics.TotalTime.Seconds())
	}
	return nil
}

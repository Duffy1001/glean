package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/duffy1001/glean/llama"
)

const (
	promptOverheadTokens = 600
	charsPerInputToken   = 3.0
)

var errPromptTooLong = errors.New("prompt exceeds context budget")

func main() {
	schemaFile := flag.String("schema", "", "JSON Schema file for constrained output")
	fields := flag.String("fields", "", "Comma-separated field names for simple schema")
	modelChoice := flag.String("model", defaultModel(), "Model: fast (0.6B)")
	maxTokens := flag.Int("max-tokens", 2048, "Maximum tokens to generate")
	compact := flag.Bool("compact", false, "Output compact JSON")
	nThreads := flag.Int("threads", 4, "CPU threads")
	nCtx := flag.Int("ctx", 8192, "Context window size")
	delimiter := flag.String("delimiter", "\\n", "Record delimiter for array extraction (supports \\n, \\t, and \\0)")
	noGrammar := flag.Bool("no-grammar", false, "Disable grammar-constrained generation")
	verbose := flag.Bool("verbose", false, "Show llama.cpp debug output")
	device := flag.String("device", "auto", "Inference device: auto, cpu, or gpu")
	gpuLayers := flag.Int("gpu-layers", -1, "Model layers to offload (-1 means all available)")
	showVersion := flag.Bool("version", false, "Show version and build edition")
	showReport := flag.Bool("report", false, "Report available inference backends and devices as JSON")
	flag.Parse()
	if *showVersion {
		fmt.Printf("glean %s (%s)\n", version, buildVariant())
		return
	}

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

	schema := defaultSchema
	if *schemaFile != "" {
		data, err := os.ReadFile(*schemaFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading schema: %v\n", err)
			os.Exit(1)
		}
		schema = string(data)
	} else if *fields != "" {
		schema = buildSchemaFromFields(*fields)
	}

	validator, err := NewSchemaValidator(schema)
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
		grammar, err = jsonSchemaToGBNF(schema)
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

	delim, err := decodeDelimiter(*delimiter)
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
	writeArrayItem := func(item interface{}) error {
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
		raw, generated, hitLimit, err := generateOne(m, schema, chunk, *maxTokens, *nCtx, eos, *verbose)
		totalGenerated += generated
		if err != nil {
			if errors.Is(err, errPromptTooLong) {
				left, right, ok := splitChunk(chunk)
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
				left, right, ok := splitChunk(chunk)
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

	arraySchema := schemaHasRootType(schema, "array")
	if arraySchema {
		hadInput, err := streamSources(args, inputCharsBudget, delim, processArrayChunk)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !hadInput {
			fmt.Fprintln(os.Stderr, "No input provided")
			os.Exit(1)
		}
		if !arrayStarted {
			fmt.Print("[]")
		} else {
			fmt.Print("]")
		}
		fmt.Println()
	} else {
		input, err := readSources(args)
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
		raw, generated, _, err := generateOne(m, schema, input, *maxTokens, *nCtx, eos, *verbose)
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

func generateOne(m *llama.Model, schema, chunkInput string, maxTokens, nCtx int, eos int32, verbose bool) (string, int, bool, error) {
	systemMsg := "You are a JSON extraction engine. Output ONLY valid JSON. No explanation, no markdown. /no_think"

	userMsg := "Extract structured data from the following input as JSON matching this schema.\n\n" +
		"When the schema defines an array, extract ALL matching items from the input, not just the first. " +
		"For line-oriented lists or tables, process every non-empty source row from first to last and emit a separate item for each row. " +
		"Do not summarize, group, combine, deduplicate, or omit repeated-looking rows. Preserve literal identifiers and numeric suffixes exactly as written.\n\n" +
		"Schema:\n" + schema + "\n\nInput:\n" + chunkInput

	prompt, err := m.ChatApplyTemplate(systemMsg, userMsg, true)
	if err != nil {
		return "", 0, false, fmt.Errorf("chat template: %w", err)
	}

	tokens, err := m.Tokenize(prompt, false, true)
	if err != nil {
		return "", 0, false, fmt.Errorf("tokenize: %w", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Prompt: %d tokens\n", len(tokens))
	}
	if len(tokens)+maxTokens > nCtx {
		return "", 0, false, fmt.Errorf("%w: prompt %d + generation %d > context %d", errPromptTooLong, len(tokens), maxTokens, nCtx)
	}

	if err := m.Decode(tokens); err != nil {
		return "", 0, false, fmt.Errorf("decode prompt: %w", err)
	}

	var result strings.Builder
	generated := 0

	for i := 0; i < maxTokens; i++ {
		tok := m.SampleNext()

		if tok == eos {
			break
		}

		piece := m.TokenToPiece(tok)
		result.WriteString(piece)
		m.AcceptToken(tok)
		generated++

		tokBatch := []int32{tok}
		if err := m.Decode(tokBatch); err != nil {
			return "", generated, false, fmt.Errorf("decode step %d: %w", i, err)
		}
	}

	raw := strings.TrimSpace(result.String())
	if raw == "" {
		return "", generated, generated == maxTokens, fmt.Errorf("empty output")
	}

	return raw, generated, generated == maxTokens, nil
}

func streamSources(paths []string, byteBudget int, delimiter string, yield func(string) error) (bool, error) {
	if len(paths) == 0 {
		return streamReaderChunks(os.Stdin, byteBudget, delimiter, yield)
	}

	hadInput := false
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return hadInput, fmt.Errorf("read %s: %w", path, err)
		}
		hadFileInput, readErr := streamReaderChunks(f, byteBudget, delimiter, yield)
		closeErr := f.Close()
		hadInput = hadInput || hadFileInput
		if readErr != nil {
			return hadInput, fmt.Errorf("read %s: %w", path, readErr)
		}
		if closeErr != nil {
			return hadInput, fmt.Errorf("close %s: %w", path, closeErr)
		}
	}
	return hadInput, nil
}

func streamReaderChunks(r io.Reader, byteBudget int, delimiter string, yield func(string) error) (bool, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	scanner.Split(splitDelimiter([]byte(delimiter)))
	var records []string
	currentBytes := 0
	hadInput := false

	emit := func() error {
		if len(records) == 0 {
			return nil
		}
		chunk := strings.Join(records, delimiter)
		if strings.TrimSpace(chunk) != "" {
			hadInput = true
			if err := yield(chunk); err != nil {
				return err
			}
		}
		records = nil
		currentBytes = 0
		return nil
	}

	for scanner.Scan() {
		record := scanner.Text()
		if record != "" {
			if len(records) > 0 && currentBytes+len(record) > byteBudget {
				if emitErr := emit(); emitErr != nil {
					return hadInput, emitErr
				}
			}
			records = append(records, record)
			currentBytes += len(record)
		}
	}
	if err := scanner.Err(); err != nil {
		return hadInput, err
	}
	if err := emit(); err != nil {
		return hadInput, err
	}
	return hadInput, nil
}

func splitDelimiter(delimiter []byte) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if index := bytes.Index(data, delimiter); index >= 0 {
			return index + len(delimiter), data[:index], nil
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
}

func decodeDelimiter(value string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			continue
		}
		if i+1 >= len(value) {
			return "", errors.New("trailing escape")
		}
		i++
		switch value[i] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case '0':
			out.WriteByte(0)
		case 'r':
			out.WriteByte('\r')
		case '\\':
			out.WriteByte('\\')
		default:
			return "", fmt.Errorf("unsupported escape \\%c", value[i])
		}
	}
	if out.Len() == 0 {
		return "", errors.New("delimiter cannot be empty")
	}
	return out.String(), nil
}

func readSources(paths []string) (string, error) {
	if len(paths) == 0 {
		data, err := io.ReadAll(os.Stdin)
		return string(data), err
	}

	var input strings.Builder
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		input.Write(data)
		input.WriteByte('\n')
	}
	return input.String(), nil
}

func splitChunk(chunk string) (string, string, bool) {
	lines := strings.Split(chunk, "\n")
	if len(lines) > 1 {
		mid := len(lines) / 2
		if mid > 0 && mid < len(lines) {
			return strings.Join(lines[:mid], "\n"), strings.Join(lines[mid:], "\n"), true
		}
	}

	runes := []rune(chunk)
	if len(runes) < 2 {
		return "", "", false
	}
	mid := len(runes) / 2
	return string(runes[:mid]), string(runes[mid:]), true
}

func schemaHasRootType(schema, wanted string) bool {
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(schema), &doc); err != nil {
		return false
	}
	switch schemaType := doc["type"].(type) {
	case string:
		return schemaType == wanted
	case []interface{}:
		for _, value := range schemaType {
			if value == wanted {
				return true
			}
		}
	}
	return false
}

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/duffy1001/glean/llama"
)

const (
	promptOverheadTokens = 600
	charsPerInputToken   = 3.0
	tokensPerOutputRec   = 35
	charsPerInputRec     = 80
	overlapLines         = 3
	safetyFactor         = 0.85
)

var errPromptTooLong = errors.New("prompt exceeds context budget")

func main() {
	schemaFile := flag.String("schema", "", "JSON Schema file for constrained output")
	fields := flag.String("fields", "", "Comma-separated field names for simple schema")
	modelChoice := flag.String("model", defaultModel(), "Model: fast (0.6B) or quality (1.7B)")
	maxTokens := flag.Int("max-tokens", 2048, "Maximum tokens to generate")
	compact := flag.Bool("compact", false, "Output compact JSON")
	nThreads := flag.Int("threads", 4, "CPU threads")
	nCtx := flag.Int("ctx", 8192, "Context window size")
	chunkLines := flag.Int("chunk-lines", 4, "Maximum input lines per array extraction chunk (0 disables)")
	noGrammar := flag.Bool("no-grammar", false, "Disable grammar-constrained generation")
	verbose := flag.Bool("verbose", false, "Show llama.cpp debug output")
	pkField := flag.String("pk", "", "Primary key field for dedup/merge (default: first field with --fields)")
	showVersion := flag.Bool("version", false, "Show version and build edition")
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

	effectivePK := *pkField

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
	m, err := llama.Load(modelPath, *nCtx, *nThreads, -1)
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

	maxRecordsByContext := float64(*nCtx-promptOverheadTokens) / (float64(charsPerInputRec)/charsPerInputToken + float64(tokensPerOutputRec))
	maxRecordsByTokens := float64(*maxTokens) / float64(tokensPerOutputRec)
	maxRecords := int(maxRecordsByContext)
	if int(maxRecordsByTokens) < maxRecords {
		maxRecords = int(maxRecordsByTokens)
	}
	if maxRecords < 1 {
		maxRecords = 1
	}
	inputCharsBudget := int(float64(maxRecords*charsPerInputRec) * safetyFactor)

	var allResults []interface{}
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
		allResults = append(allResults, parsed...)
		return nil
	}

	arraySchema := schemaHasRootType(schema, "array")
	if arraySchema {
		overlap := 0
		if effectivePK != "" {
			overlap = overlapLines
		}
		hadInput, err := streamSources(args, inputCharsBudget, *chunkLines, overlap, processArrayChunk)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !hadInput {
			fmt.Fprintln(os.Stderr, "No input provided")
			os.Exit(1)
		}
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
		allResults = append(allResults, parsed)
	}

	if totalGenerated > 0 {
		verbosef("Generated %d tokens in %v (%.1f tok/s)\n",
			totalGenerated, time.Since(genStart), float64(totalGenerated)/time.Since(genStart).Seconds())
	}

	if len(allResults) == 0 && !arraySchema {
		fmt.Fprintf(os.Stderr, "No output generated\n")
		os.Exit(1)
	}

	var finalParsed interface{} = allResults
	if !arraySchema {
		finalParsed = allResults[0]
	}

	if effectivePK != "" {
		if arr, ok := finalParsed.([]interface{}); ok {
			before := len(arr)
			finalParsed = dedupByPK(arr, effectivePK)
			after := len(finalParsed.([]interface{}))
			if after < before {
				verbosef("Deduplicated %d -> %d records by %q\n", before, after, effectivePK)
			}
		}
	}

	if err := validator.Validate(string(mustMarshal(finalParsed))); err != nil {
		fmt.Fprintf(os.Stderr, "Validation error: %v\n", err)
		os.Exit(1)
	}

	var out []byte
	if *compact {
		out, err = json.Marshal(finalParsed)
	} else {
		out, err = json.MarshalIndent(finalParsed, "", "  ")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(out))
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

func streamSources(paths []string, byteBudget, maxLines, overlap int, yield func(string) error) (bool, error) {
	if len(paths) == 0 {
		return streamReaderChunks(os.Stdin, byteBudget, maxLines, overlap, yield)
	}

	hadInput := false
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return hadInput, fmt.Errorf("read %s: %w", path, err)
		}
		hadFileInput, readErr := streamReaderChunks(f, byteBudget, maxLines, overlap, yield)
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

func streamReaderChunks(r io.Reader, byteBudget, maxLines, overlap int, yield func(string) error) (bool, error) {
	reader := bufio.NewReader(r)
	var lines []string
	currentBytes := 0
	hadInput := false

	emit := func() error {
		if len(lines) == 0 {
			return nil
		}
		chunk := strings.Join(lines, "")
		if strings.TrimSpace(chunk) != "" {
			hadInput = true
			if err := yield(chunk); err != nil {
				return err
			}
		}
		keep := overlap
		if keep > len(lines) {
			keep = len(lines)
		}
		lines = append([]string(nil), lines[len(lines)-keep:]...)
		currentBytes = 0
		for _, line := range lines {
			currentBytes += len(line)
		}
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if len(lines) > 0 && (currentBytes+len(line) > byteBudget || (maxLines > 0 && len(lines) >= maxLines)) {
				if emitErr := emit(); emitErr != nil {
					return hadInput, emitErr
				}
			}
			lines = append(lines, line)
			currentBytes += len(line)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return hadInput, err
			}
			break
		}
	}
	if len(lines) > 0 {
		chunk := strings.Join(lines, "")
		if strings.TrimSpace(chunk) != "" {
			hadInput = true
			if err := yield(chunk); err != nil {
				return hadInput, err
			}
		}
	}
	return hadInput, nil
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

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

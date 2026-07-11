package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/duffy1001/glean/llama"
)

const (
	promptOverheadTokens = 400
	charsPerToken        = 3.5
	overlapLines         = 3
)

func main() {
	schemaFile := flag.String("schema", "", "JSON Schema file for constrained output")
	fields := flag.String("fields", "", "Comma-separated field names for simple schema")
	modelChoice := flag.String("model", "fast", "Model: fast (0.6B) or quality (1.7B)")
	maxTokens := flag.Int("max-tokens", 512, "Maximum tokens to generate")
	compact := flag.Bool("compact", false, "Output compact JSON")
	nThreads := flag.Int("threads", 4, "CPU threads")
	nCtx := flag.Int("ctx", 2048, "Context window size")
	noGrammar := flag.Bool("no-grammar", false, "Disable grammar-constrained generation")
	verbose := flag.Bool("verbose", false, "Show llama.cpp debug output")
	pkField := flag.String("pk", "", "Primary key field for dedup/merge (default: first field with --fields)")
	flag.Parse()

	if *verbose {
		llama.SetLogLevel(1)
	} else {
		llama.SetLogLevel(3)
	}

	var input string
	args := flag.Args()
	if len(args) > 0 {
		for _, arg := range args {
			data, err := os.ReadFile(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", arg, err)
				os.Exit(1)
			}
			input += string(data) + "\n"
		}
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		input = string(data)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		fmt.Fprintf(os.Stderr, "No input provided\n")
		os.Exit(1)
	}

	schema := defaultSchema
	usingFields := false
	if *schemaFile != "" {
		data, err := os.ReadFile(*schemaFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading schema: %v\n", err)
			os.Exit(1)
		}
		schema = string(data)
	} else if *fields != "" {
		schema = buildSchemaFromFields(*fields)
		usingFields = true
	}

	effectivePK := *pkField
	if effectivePK == "" && usingFields {
		for _, f := range strings.Split(*fields, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				effectivePK = f
				break
			}
		}
	}

	validator, err := NewSchemaValidator(schema)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Schema error: %v\n", err)
		os.Exit(1)
	}

	modelPath, err := resolveModel(*modelChoice)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Loading model (%s)...\n", *modelChoice)
	start := time.Now()
	m, err := llama.Load(modelPath, *nCtx, *nThreads, -1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading model: %v\n", err)
		os.Exit(1)
	}
	defer m.Free()
	fmt.Fprintf(os.Stderr, "Model loaded in %v\n", time.Since(start))

	if !*noGrammar {
		gbnf, err := jsonSchemaToGBNF(schema)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Schema conversion error: %v\n", err)
			os.Exit(1)
		}
		if err := m.SetGrammar(gbnf, "root"); err != nil {
			fmt.Fprintf(os.Stderr, "Grammar error: %v\n", err)
			os.Exit(1)
		}
	}

	eos := m.TokenEOS()

	inputBudget := *nCtx - promptOverheadTokens - *maxTokens
	inputCharsBudget := int(float64(inputBudget) * charsPerToken)

	chunks := chunkInput(input, inputCharsBudget)
	totalChunks := len(chunks)

	var allResults []json.RawMessage
	totalGenerated := 0
	genStart := time.Now()

	for ci, chunk := range chunks {
		if totalChunks > 1 {
			fmt.Fprintf(os.Stderr, "Processing chunk %d/%d (%d bytes)...\n", ci+1, totalChunks, len(chunk))
		}

		raw, generated, err := generateOne(m, schema, chunk, *maxTokens, eos)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Chunk %d error: %v\n", ci+1, err)
			continue
		}
		totalGenerated += generated

		var parsed interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			fmt.Fprintf(os.Stderr, "Chunk %d: invalid JSON: %v\n", ci+1, err)
			continue
		}

		switch v := parsed.(type) {
		case []interface{}:
			for _, item := range v {
				b, _ := json.Marshal(item)
				allResults = append(allResults, b)
			}
		case map[string]interface{}:
			allResults = append(allResults, json.RawMessage(raw))
		}

		if totalChunks > 1 && ci < totalChunks-1 {
			m.ClearContext()
			if !*noGrammar {
				gbnf, _ := jsonSchemaToGBNF(schema)
				m.SetGrammar(gbnf, "root")
			}
		}
	}

	if totalGenerated > 0 {
		fmt.Fprintf(os.Stderr, "Generated %d tokens in %v (%.1f tok/s)\n",
			totalGenerated, time.Since(genStart), float64(totalGenerated)/time.Since(genStart).Seconds())
	}

	if len(allResults) == 0 {
		fmt.Fprintf(os.Stderr, "No output generated\n")
		os.Exit(1)
	}

	var finalParsed interface{}
	if len(allResults) == 1 {
		json.Unmarshal(allResults[0], &finalParsed)
	} else {
		finalParsed = allResults
	}

	if effectivePK != "" {
		if arr, ok := finalParsed.([]interface{}); ok {
			before := len(arr)
			finalParsed = dedupByPK(arr, effectivePK)
			after := len(finalParsed.([]interface{}))
			if after < before {
				fmt.Fprintf(os.Stderr, "Deduplicated %d -> %d records by %q\n", before, after, effectivePK)
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

func generateOne(m *llama.Model, schema, chunkInput string, maxTokens int, eos int32) (string, int, error) {
	systemMsg := "You are a JSON extraction engine. Output ONLY valid JSON. No explanation, no markdown. /no_think"

	userMsg := "Extract structured data from the following input as JSON matching this schema.\n\n" +
		"When the schema defines an array, extract ALL matching items from the input, not just the first.\n\n" +
		"Schema:\n" + schema + "\n\nInput:\n" + chunkInput

	prompt, err := m.ChatApplyTemplate(systemMsg, userMsg, true)
	if err != nil {
		return "", 0, fmt.Errorf("chat template: %w", err)
	}

	tokens, err := m.Tokenize(prompt, false, true)
	if err != nil {
		return "", 0, fmt.Errorf("tokenize: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Prompt: %d tokens\n", len(tokens))

	if err := m.Decode(tokens); err != nil {
		return "", 0, fmt.Errorf("decode prompt: %w", err)
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
			return "", generated, fmt.Errorf("decode step %d: %w", i, err)
		}
	}

	raw := strings.TrimSpace(result.String())
	if raw == "" {
		return "", generated, fmt.Errorf("empty output")
	}

	return raw, generated, nil
}

func chunkInput(input string, charBudget int) []string {
	lines := strings.Split(input, "\n")

	if estimateChars(input) <= charBudget {
		return []string{input}
	}

	var chunks []string
	var current []string
	currentLen := 0

	for _, line := range lines {
		lineLen := len(line) + 1

		if currentLen+lineLen > charBudget && len(current) > 0 {
			chunks = append(chunks, strings.Join(current, "\n"))

			overlap := overlapLines
			if overlap > len(current) {
				overlap = len(current)
			}
			current = current[len(current)-overlap:]
			currentLen = 0
			for _, l := range current {
				currentLen += len(l) + 1
			}
		}

		current = append(current, line)
		currentLen += lineLen
	}

	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, "\n"))
	}

	return chunks
}

func estimateChars(s string) int {
	return len(s)
}

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

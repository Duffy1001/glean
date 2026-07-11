package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/duffy/glean/llama"
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

	systemMsg := "You are a JSON extraction engine. Output ONLY valid JSON. No explanation, no markdown. /no_think"

	userMsg := "Extract structured data from the following input as JSON matching this schema.\n\n" +
		"When the schema defines an array, extract ALL matching items from the input, not just the first.\n\n" +
		"Schema:\n" + schema + "\n\nInput:\n" + input

	prompt, err := m.ChatApplyTemplate(systemMsg, userMsg, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Chat template error: %v\n", err)
		os.Exit(1)
	}

	eos := m.TokenEOS()

	tokens, err := m.Tokenize(prompt, false, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Tokenize error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Prompt: %d tokens\n", len(tokens))

	if err := m.Decode(tokens); err != nil {
		fmt.Fprintf(os.Stderr, "Decode error: %v\n", err)
		os.Exit(1)
	}

	var result strings.Builder
	generated := 0
	genStart := time.Now()

	for i := 0; i < *maxTokens; i++ {
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
			fmt.Fprintf(os.Stderr, "Decode error at step %d: %v\n", i, err)
			os.Exit(1)
		}
	}

	if generated > 0 {
		fmt.Fprintf(os.Stderr, "Generated %d tokens in %v (%.1f tok/s)\n",
			generated, time.Since(genStart), float64(generated)/time.Since(genStart).Seconds())
	}

	raw := strings.TrimSpace(result.String())
	if raw == "" {
		fmt.Fprintf(os.Stderr, "No output generated\n")
		os.Exit(1)
	}

	if err := validator.Validate(raw); err != nil {
		fmt.Fprintf(os.Stderr, "Validation error: %v\nRaw output: %s\n", err, raw)
		os.Exit(1)
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid JSON output: %v\nRaw: %s\n", err, raw)
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

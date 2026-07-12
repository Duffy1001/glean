package extract

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/duffy1001/glean/llama"
)

var errPromptTooLong = errors.New("prompt exceeds context budget")

type generationOutput struct {
	raw      string
	hitLimit bool
	metrics  Metrics
}

func generateOne(ctx context.Context, m *llama.Model, schema, chunkInput string, maxTokens, nCtx int, eos int32) (generationOutput, error) {
	if err := ctx.Err(); err != nil {
		return generationOutput{}, err
	}
	var output generationOutput
	promptStart := time.Now()
	systemMsg := "You are a JSON extraction engine. Output ONLY valid JSON. No explanation, no markdown. /no_think"

	userMsg := "Extract structured data from the following input as JSON matching this schema.\n\n" +
		"When the schema defines an array, extract ALL matching items from the input, not just the first. " +
		"For line-oriented lists or tables, process every non-empty source row from first to last and emit a separate item for each row. " +
		"Do not summarize, group, combine, deduplicate, or omit repeated-looking rows. Preserve literal identifiers and numeric suffixes exactly as written.\n\n" +
		"Schema:\n" + schema + "\n\nInput:\n" + chunkInput

	prompt, err := m.ChatApplyTemplate(systemMsg, userMsg, true)
	if err != nil {
		return output, fmt.Errorf("chat template: %w", err)
	}
	output.metrics.PromptBuildTime = time.Since(promptStart)

	tokenizeStart := time.Now()
	tokens, err := m.Tokenize(prompt, false, true)
	if err != nil {
		return output, fmt.Errorf("tokenize: %w", err)
	}
	output.metrics.TokenizeTime = time.Since(tokenizeStart)
	output.metrics.PromptTokens = len(tokens)
	if len(tokens)+maxTokens > nCtx {
		return output, fmt.Errorf("%w: prompt %d + generation %d > context %d", errPromptTooLong, len(tokens), maxTokens, nCtx)
	}

	prefillStart := time.Now()
	if err := m.Decode(tokens); err != nil {
		return output, fmt.Errorf("decode prompt: %w", err)
	}
	output.metrics.PrefillTime = time.Since(prefillStart)

	var result strings.Builder
	generationStart := time.Now()

	for i := 0; i < maxTokens; i++ {
		if i%16 == 0 {
			if err := ctx.Err(); err != nil {
				output.metrics.GenerationTime = time.Since(generationStart)
				return output, err
			}
		}
		tok := m.SampleNext()
		if i == 0 {
			output.metrics.TimeToFirstToken = time.Since(generationStart)
		}

		if tok == eos {
			break
		}

		piece := m.TokenToPiece(tok)
		result.WriteString(piece)
		m.AcceptToken(tok)
		output.metrics.GeneratedTokens++

		tokBatch := []int32{tok}
		if err := m.Decode(tokBatch); err != nil {
			output.metrics.GenerationTime = time.Since(generationStart)
			return output, fmt.Errorf("decode step %d: %w", i, err)
		}
	}
	output.metrics.GenerationTime = time.Since(generationStart)

	output.raw = strings.TrimSpace(result.String())
	output.hitLimit = output.metrics.GeneratedTokens == maxTokens
	if output.raw == "" {
		return output, fmt.Errorf("empty output")
	}
	return output, nil
}

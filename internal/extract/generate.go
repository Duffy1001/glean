package extract

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/duffy1001/glean/llama"
)

var errPromptTooLong = errors.New("prompt exceeds context budget")

func generateOne(ctx context.Context, m *llama.Model, schema, chunkInput string, maxTokens, nCtx int, eos int32, verbose bool) (string, int, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, false, err
	}
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
		if i%16 == 0 {
			if err := ctx.Err(); err != nil {
				return "", generated, false, err
			}
		}
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

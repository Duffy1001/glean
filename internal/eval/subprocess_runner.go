package eval

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SubprocessConfig struct {
	Binary      string
	Threads     int
	ContextSize int
	Device      string
	GPULayers   int
	NoGrammar   bool
	MaxTokens   int
	Compact     bool
}

func RunSubprocess(ctx context.Context, corpus Corpus, cfg SubprocessConfig, repetitions int) ([]Sample, error) {
	if repetitions < 1 {
		return nil, fmt.Errorf("repetitions must be at least 1")
	}
	if cfg.Binary == "" {
		return nil, fmt.Errorf("subprocess binary is required")
	}
	if cfg.MaxTokens < 1 {
		return nil, fmt.Errorf("max-tokens must be at least 1")
	}

	samples := make([]Sample, 0, len(corpus.Cases)*repetitions)
	for _, c := range corpus.Cases {
		if err := ctx.Err(); err != nil {
			return samples, err
		}

		schemaPath := filepath.Join(c.CaseDir, "schema.json")
		delimiterArg := encodeDelimiterForCLI(c.Case.Delimiter)

		for repetition := 0; repetition < repetitions; repetition++ {
			if err := ctx.Err(); err != nil {
				return samples, err
			}

			args := []string{
				"--schema", schemaPath,
				"--delimiter", delimiterArg,
				"--max-tokens", strconv.Itoa(cfg.MaxTokens),
				"--ctx", strconv.Itoa(cfg.ContextSize),
				"--threads", strconv.Itoa(cfg.Threads),
				"--device", cfg.Device,
				"--gpu-layers", strconv.Itoa(cfg.GPULayers),
			}
			if cfg.NoGrammar {
				args = append(args, "--no-grammar")
			}
			if cfg.Compact {
				args = append(args, "--compact")
			}

			start := time.Now()
			cmd := exec.CommandContext(ctx, cfg.Binary, args...)
			cmd.Stdin = bytes.NewReader(c.Input)
			var stdoutBuf, stderrBuf bytes.Buffer
			cmd.Stdout = &stdoutBuf
			cmd.Stderr = &stderrBuf
			err := cmd.Run()
			stdout := stdoutBuf.Bytes()
			stderr := strings.TrimSpace(stderrBuf.String())

			sample := Sample{
				CaseID:     c.Case.ID,
				Repetition: repetition,
				WallTime:   time.Since(start),
			}
			sample.Score = ScoreCase(c, stdout)
			sample.Valid = err == nil && sample.Score.ExactMatch
			if err != nil {
				sample.Error = err.Error()
				if stderr != "" {
					sample.Error = sample.Error + ": " + stderr
				}
			}
			samples = append(samples, sample)
		}
	}
	return samples, nil
}

func encodeDelimiterForCLI(delimiter string) string {
	// glean's CLI uses extract.DecodeDelimiter, which expects escapes in a single
	// argument string (e.g. "\\n" for a newline). Here we invert that mapping.
	//
	// The returned value contains no literal newlines/tabs/NUL bytes, so it is
	// safe to pass as a process argument.
	var b strings.Builder
	b.Grow(len(delimiter) * 2)
	for i := 0; i < len(delimiter); i++ {
		switch delimiter[i] {
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case 0:
			b.WriteString(`\0`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteByte(delimiter[i])
		}
	}
	return b.String()
}

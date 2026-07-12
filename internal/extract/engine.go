package extract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/duffy1001/glean/llama"
)

var ErrNoInput = errors.New("no input provided")

const (
	promptOverheadTokens = 600
	charsPerInputToken   = 3.0
)

type Engine struct {
	mu     sync.Mutex
	model  *llama.Model
	config Config
	eos    int32
	closed bool
}

func NewEngine(ctx context.Context, cfg Config) (*Engine, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := InitializeBackend(); err != nil {
		return nil, err
	}

	m, err := llama.Load(cfg.ModelPath, cfg.ContextSize, cfg.Threads, cfg.GPULayers, cfg.AllowCPUFallback)
	if err != nil {
		return nil, err
	}
	return &Engine{model: m, config: cfg, eos: m.TokenEOS()}, nil
}

func (e *Engine) Extract(ctx context.Context, req Request, sources []Source) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return Result{}, errors.New("extract with closed engine")
	}

	prepared, err := prepareSchema(req.Schema, e.config.GrammarEnabled)
	if err != nil {
		return Result{}, err
	}

	start := time.Now()
	if prepared.root == rootArray {
		return e.extractArray(ctx, prepared, req, sources, start)
	}
	return e.extractObject(ctx, prepared, req, sources, start)
}

func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.model.Free()
	e.model = nil
	e.closed = true
	return nil
}

func (e *Engine) extractObject(ctx context.Context, prepared *preparedSchema, req Request, sources []Source, start time.Time) (Result, error) {
	input, err := readSources(sources)
	if err != nil {
		return Result{}, err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return Result{}, ErrNoInput
	}
	if err := e.reset(prepared); err != nil {
		return Result{}, err
	}

	raw, generated, _, err := generateOne(ctx, e.model, prepared.raw, input, req.MaxTokens, e.config.ContextSize, e.eos)
	if err != nil {
		return Result{}, err
	}
	if err := prepared.validator.validate(raw); err != nil {
		return Result{}, err
	}
	return Result{JSON: json.RawMessage(raw), GeneratedTokens: generated, TotalTime: time.Since(start)}, nil
}

func (e *Engine) extractArray(ctx context.Context, prepared *preparedSchema, req Request, sources []Source, start time.Time) (Result, error) {
	items := make([]json.RawMessage, 0)
	generatedTokens := 0
	chunkNumber := 0

	var processChunk func(string) error
	processChunk = func(chunk string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			return nil
		}
		if err := e.reset(prepared); err != nil {
			return err
		}
		chunkNumber++
		raw, generated, hitLimit, err := generateOne(ctx, e.model, prepared.raw, chunk, req.MaxTokens, e.config.ContextSize, e.eos)
		generatedTokens += generated
		if err != nil {
			if errors.Is(err, errPromptTooLong) {
				left, right, ok := splitChunk(chunk, req.Delimiter)
				if ok {
					if err := processChunk(left); err != nil {
						return err
					}
					return processChunk(right)
				}
			}
			return fmt.Errorf("chunk %d: %w", chunkNumber, err)
		}

		var parsed []json.RawMessage
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			if hitLimit {
				left, right, ok := splitChunk(chunk, req.Delimiter)
				if ok {
					if err := processChunk(left); err != nil {
						return err
					}
					return processChunk(right)
				}
			}
			return fmt.Errorf("chunk %d produced invalid JSON: %w", chunkNumber, err)
		}
		if err := prepared.validator.validate(raw); err != nil {
			return fmt.Errorf("chunk %d validation: %w", chunkNumber, err)
		}
		items = append(items, parsed...)
		return nil
	}

	inputBudget := e.config.ContextSize - promptOverheadTokens - req.MaxTokens
	if inputBudget < 1 {
		return Result{}, errors.New("context is too small for the requested max-tokens")
	}
	inputCharsBudget := int(float64(inputBudget) * charsPerInputToken)
	hadInput, err := streamSources(ctx, sources, inputCharsBudget, req.Delimiter, processChunk)
	if err != nil {
		return Result{}, err
	}
	if !hadInput {
		return Result{}, ErrNoInput
	}

	raw, err := json.Marshal(items)
	if err != nil {
		return Result{}, fmt.Errorf("marshal array result: %w", err)
	}
	if err := prepared.validator.validate(string(raw)); err != nil {
		return Result{}, err
	}
	return Result{JSON: raw, GeneratedTokens: generatedTokens, TotalTime: time.Since(start)}, nil
}

func (e *Engine) reset(prepared *preparedSchema) error {
	if err := e.model.ClearContext(); err != nil {
		return err
	}
	if prepared.grammar == "" {
		e.model.ClearGrammar()
		return nil
	}
	if err := e.model.SetGrammar(prepared.grammar, "root"); err != nil {
		return fmt.Errorf("reset grammar: %w", err)
	}
	return nil
}

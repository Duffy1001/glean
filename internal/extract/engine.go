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

	start := time.Now()
	prepared, err := prepareSchema(req.Schema, e.config.GrammarEnabled)
	if err != nil {
		return Result{Metrics: Metrics{TotalTime: time.Since(start)}}, err
	}

	if prepared.root == rootArray {
		return e.extractArray(ctx, prepared, req, sources, start)
	}
	return e.extractObject(ctx, prepared, req, sources, start)
}

// ExtractStream streams root-array outputs to arraySink as each chunk succeeds.
//
// For root-array schemas, Result.JSON is left unset and arraySink is responsible
// for writing a complete JSON array to the caller.
// For root-object schemas, this behaves like Extract.
func (e *Engine) ExtractStream(ctx context.Context, req Request, sources []Source, arraySink ArraySink) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return Result{}, errors.New("extract with closed engine")
	}

	start := time.Now()
	prepared, err := prepareSchema(req.Schema, e.config.GrammarEnabled)
	if err != nil {
		return Result{Metrics: Metrics{TotalTime: time.Since(start)}}, err
	}

	if prepared.root == rootArray {
		if arraySink == nil {
			return e.extractArray(ctx, prepared, req, sources, start)
		}
		return e.extractArrayStream(ctx, prepared, req, sources, start, arraySink)
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
	var metrics Metrics
	input, err := readSources(sources)
	if err != nil {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics}, err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics}, ErrNoInput
	}
	metrics.InputBytes = int64(len(input))
	if err := e.reset(prepared); err != nil {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics}, err
	}

	generation, err := generateOne(ctx, e.model, prepared.raw, input, req.MaxTokens, e.config.ContextSize, e.eos)
	if err != nil {
		metrics = generation.metrics
		metrics.InputBytes = int64(len(input))
		metrics.ChunksProcessed = 1
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics}, err
	}
	metrics = generation.metrics
	metrics.InputBytes = int64(len(input))
	metrics.ChunksProcessed = 1

	parseStart := time.Now()
	parsed, err := parseJSON(generation.raw)
	metrics.ParseTime = time.Since(parseStart)
	if err != nil {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics}, err
	}
	validationStart := time.Now()
	if err := prepared.validator.validateValue(parsed); err != nil {
		metrics.ValidationTime = time.Since(validationStart)
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics}, err
	}
	metrics.ValidationTime = time.Since(validationStart)
	metrics.RecordsProduced = 1
	metrics.OutputBytes = int64(len(generation.raw))
	metrics.TotalTime = time.Since(start)
	chunk := ChunkMetrics{
		Index:            1,
		InputBytes:       metrics.InputBytes,
		PromptTokens:     metrics.PromptTokens,
		GeneratedTokens:  metrics.GeneratedTokens,
		RecordsProduced:  1,
		PromptBuildTime:  metrics.PromptBuildTime,
		TokenizeTime:     metrics.TokenizeTime,
		PrefillTime:      metrics.PrefillTime,
		TimeToFirstToken: metrics.TimeToFirstToken,
		GenerationTime:   metrics.GenerationTime,
		ParseTime:        metrics.ParseTime,
		ValidationTime:   metrics.ValidationTime,
		TotalTime:        time.Since(start),
	}
	return Result{JSON: json.RawMessage(generation.raw), Metrics: metrics, ChunkRuns: []ChunkMetrics{chunk}}, nil
}

func (e *Engine) extractArray(ctx context.Context, prepared *preparedSchema, req Request, sources []Source, start time.Time) (Result, error) {
	items := make([]any, 0)
	var metrics Metrics
	chunkRuns := make([]ChunkMetrics, 0)
	chunkNumber := 0

	var processChunk func(string, int) error
	processChunk = func(chunk string, retryDepth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			return nil
		}
		chunkNumber++
		chunkMetrics := ChunkMetrics{
			Index:      chunkNumber,
			InputBytes: int64(len(chunk)),
			Retried:    retryDepth > 0,
			RetryDepth: retryDepth,
		}
		chunkStart := time.Now()
		runIndex := len(chunkRuns)
		chunkRuns = append(chunkRuns, chunkMetrics)
		defer func() {
			chunkMetrics.TotalTime = time.Since(chunkStart)
			metrics.addChunk(chunkMetrics)
			chunkRuns[runIndex] = chunkMetrics
		}()

		if err := e.reset(prepared); err != nil {
			return err
		}
		generation, err := generateOne(ctx, e.model, prepared.raw, chunk, req.MaxTokens, e.config.ContextSize, e.eos)
		chunkMetrics.PromptTokens = generation.metrics.PromptTokens
		chunkMetrics.GeneratedTokens = generation.metrics.GeneratedTokens
		chunkMetrics.PromptBuildTime = generation.metrics.PromptBuildTime
		chunkMetrics.TokenizeTime = generation.metrics.TokenizeTime
		chunkMetrics.PrefillTime = generation.metrics.PrefillTime
		chunkMetrics.TimeToFirstToken = generation.metrics.TimeToFirstToken
		chunkMetrics.GenerationTime = generation.metrics.GenerationTime
		if err != nil {
			if errors.Is(err, errPromptTooLong) {
				left, right, ok := splitChunk(chunk, req.Delimiter)
				if ok {
					if err := processChunk(left, retryDepth+1); err != nil {
						return err
					}
					return processChunk(right, retryDepth+1)
				}
			}
			return fmt.Errorf("chunk %d: %w", chunkNumber, err)
		}

		parseStart := time.Now()
		parsed, err := parseJSON(generation.raw)
		chunkMetrics.ParseTime = time.Since(parseStart)
		if err != nil {
			if generation.hitLimit {
				left, right, ok := splitChunk(chunk, req.Delimiter)
				if ok {
					if err := processChunk(left, retryDepth+1); err != nil {
						return err
					}
					return processChunk(right, retryDepth+1)
				}
			}
			return fmt.Errorf("chunk %d produced invalid JSON: %w", chunkNumber, err)
		}
		array, ok := parsed.([]any)
		if !ok {
			return fmt.Errorf("chunk %d produced a non-array result", chunkNumber)
		}
		validationStart := time.Now()
		if err := prepared.validator.validateValue(parsed); err != nil {
			chunkMetrics.ValidationTime = time.Since(validationStart)
			return fmt.Errorf("chunk %d validation: %w", chunkNumber, err)
		}
		chunkMetrics.ValidationTime = time.Since(validationStart)
		chunkMetrics.RecordsProduced = len(array)
		items = append(items, array...)
		return nil
	}

	inputBudget := e.config.ContextSize - promptOverheadTokens - req.MaxTokens
	if inputBudget < 1 {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, errors.New("context is too small for the requested max-tokens")
	}
	inputCharsBudget := int(float64(inputBudget) * charsPerInputToken)
	hadInput, err := streamSources(ctx, sources, inputCharsBudget, req.Delimiter, func(chunk string) error {
		return processChunk(chunk, 0)
	})
	if err != nil {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, err
	}
	if !hadInput {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, ErrNoInput
	}

	serializationStart := time.Now()
	raw, err := json.Marshal(items)
	metrics.SerializationTime = time.Since(serializationStart)
	if err != nil {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, fmt.Errorf("marshal array result: %w", err)
	}
	validationStart := time.Now()
	if err := prepared.validator.validateValue(items); err != nil {
		metrics.ValidationTime += time.Since(validationStart)
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, err
	}
	metrics.ValidationTime += time.Since(validationStart)
	metrics.OutputBytes = int64(len(raw))
	metrics.RecordsProduced = len(items)
	metrics.TotalTime = time.Since(start)
	return Result{JSON: raw, Metrics: metrics, ChunkRuns: chunkRuns}, nil
}

func (e *Engine) extractArrayStream(ctx context.Context, prepared *preparedSchema, req Request, sources []Source, start time.Time, sink ArraySink) (Result, error) {
	items := make([]any, 0)
	var metrics Metrics
	chunkRuns := make([]ChunkMetrics, 0)
	chunkNumber := 0

	streamStarted := false
	startSink := func() error {
		if streamStarted {
			return nil
		}
		if err := sink.Start(); err != nil {
			return err
		}
		streamStarted = true
		return nil
	}
	finishSink := func() error {
		if !streamStarted {
			if err := sink.Start(); err != nil {
				return err
			}
			streamStarted = true
		}
		return sink.Finish()
	}

	var processChunk func(string, int) error
	processChunk = func(chunk string, retryDepth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			return nil
		}
		chunkNumber++
		chunkMetrics := ChunkMetrics{
			Index:      chunkNumber,
			InputBytes: int64(len(chunk)),
			Retried:    retryDepth > 0,
			RetryDepth: retryDepth,
		}
		chunkStart := time.Now()
		runIndex := len(chunkRuns)
		chunkRuns = append(chunkRuns, chunkMetrics)
		defer func() {
			chunkMetrics.TotalTime = time.Since(chunkStart)
			metrics.addChunk(chunkMetrics)
			chunkRuns[runIndex] = chunkMetrics
		}()

		if err := e.reset(prepared); err != nil {
			return err
		}
		generation, err := generateOne(ctx, e.model, prepared.raw, chunk, req.MaxTokens, e.config.ContextSize, e.eos)
		chunkMetrics.PromptTokens = generation.metrics.PromptTokens
		chunkMetrics.GeneratedTokens = generation.metrics.GeneratedTokens
		chunkMetrics.PromptBuildTime = generation.metrics.PromptBuildTime
		chunkMetrics.TokenizeTime = generation.metrics.TokenizeTime
		chunkMetrics.PrefillTime = generation.metrics.PrefillTime
		chunkMetrics.TimeToFirstToken = generation.metrics.TimeToFirstToken
		chunkMetrics.GenerationTime = generation.metrics.GenerationTime
		if err != nil {
			if errors.Is(err, errPromptTooLong) {
				left, right, ok := splitChunk(chunk, req.Delimiter)
				if ok {
					if err := processChunk(left, retryDepth+1); err != nil {
						return err
					}
					return processChunk(right, retryDepth+1)
				}
			}
			return fmt.Errorf("chunk %d: %w", chunkNumber, err)
		}

		parseStart := time.Now()
		parsed, err := parseJSON(generation.raw)
		chunkMetrics.ParseTime = time.Since(parseStart)
		if err != nil {
			if generation.hitLimit {
				left, right, ok := splitChunk(chunk, req.Delimiter)
				if ok {
					if err := processChunk(left, retryDepth+1); err != nil {
						return err
					}
					return processChunk(right, retryDepth+1)
				}
			}
			return fmt.Errorf("chunk %d produced invalid JSON: %w", chunkNumber, err)
		}
		array, ok := parsed.([]any)
		if !ok {
			return fmt.Errorf("chunk %d produced a non-array result", chunkNumber)
		}
		validationStart := time.Now()
		if err := prepared.validator.validateValue(parsed); err != nil {
			chunkMetrics.ValidationTime = time.Since(validationStart)
			return fmt.Errorf("chunk %d validation: %w", chunkNumber, err)
		}
		chunkMetrics.ValidationTime = time.Since(validationStart)
		chunkMetrics.RecordsProduced = len(array)

		if len(array) > 0 {
			if err := startSink(); err != nil {
				return fmt.Errorf("sink start: %w", err)
			}
			for _, item := range array {
				if err := sink.WriteItem(item); err != nil {
					return fmt.Errorf("sink write: %w", err)
				}
			}
		}

		items = append(items, array...)
		return nil
	}

	inputBudget := e.config.ContextSize - promptOverheadTokens - req.MaxTokens
	if inputBudget < 1 {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, errors.New("context is too small for the requested max-tokens")
	}
	inputCharsBudget := int(float64(inputBudget) * charsPerInputToken)
	hadInput, err := streamSources(ctx, sources, inputCharsBudget, req.Delimiter, func(chunk string) error {
		return processChunk(chunk, 0)
	})
	if err != nil {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, err
	}
	if !hadInput {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, ErrNoInput
	}

	serializationStart := time.Now()
	raw, err := json.Marshal(items)
	metrics.SerializationTime = time.Since(serializationStart)
	if err != nil {
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, fmt.Errorf("marshal array result: %w", err)
	}
	validationStart := time.Now()
	if err := prepared.validator.validateValue(items); err != nil {
		metrics.ValidationTime += time.Since(validationStart)
		metrics.TotalTime = time.Since(start)
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, err
	}
	metrics.ValidationTime += time.Since(validationStart)
	metrics.OutputBytes = int64(len(raw))
	metrics.RecordsProduced = len(items)
	metrics.TotalTime = time.Since(start)

	if err := finishSink(); err != nil {
		return Result{Metrics: metrics, ChunkRuns: chunkRuns}, fmt.Errorf("sink finish: %w", err)
	}
	return Result{Metrics: metrics, ChunkRuns: chunkRuns}, nil
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

package eval

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/duffy1001/glean/internal/extract"
)

type Sample struct {
	CaseID     string          `json:"case_id"`
	Repetition int             `json:"repetition"`
	Valid      bool            `json:"valid"`
	Score      CaseScore       `json:"score"`
	Metrics    extract.Metrics `json:"metrics"`
	WallTime   time.Duration   `json:"wall_time_ns"`
	Error      string          `json:"error,omitempty"`
}

func RunQuality(ctx context.Context, corpus Corpus, engine *extract.Engine, repetitions int) ([]Sample, error) {
	if repetitions < 1 {
		return nil, fmt.Errorf("repetitions must be at least 1")
	}
	samples := make([]Sample, 0, len(corpus.Cases)*repetitions)
	for _, c := range corpus.Cases {
		for repetition := 0; repetition < repetitions; repetition++ {
			if err := ctx.Err(); err != nil {
				return samples, err
			}
			start := time.Now()
			result, err := engine.Extract(ctx, extract.Request{
				Schema:    string(c.Schema),
				MaxTokens: 2048,
				Delimiter: c.Case.Delimiter,
			}, []extract.Source{{Name: c.Case.ID, Reader: bytes.NewReader(c.Input)}})
			sample := Sample{
				CaseID:     c.Case.ID,
				Repetition: repetition,
				Metrics:    result.Metrics,
				WallTime:   time.Since(start),
			}
			if err != nil {
				sample.Score = ScoreCase(c, nil)
				sample.Error = err.Error()
			} else {
				sample.Score = ScoreCase(c, result.JSON)
				sample.Valid = sample.Score.ExactMatch
			}
			samples = append(samples, sample)
		}
	}
	return samples, nil
}

func RunWarm(ctx context.Context, corpus Corpus, engine *extract.Engine, repetitions, warmups int) ([]Sample, error) {
	if repetitions < 1 {
		return nil, fmt.Errorf("repetitions must be at least 1")
	}
	if warmups < 0 {
		return nil, fmt.Errorf("warmups must be >= 0")
	}
	samples := make([]Sample, 0, len(corpus.Cases)*repetitions)
	for _, c := range corpus.Cases {
		for warmup := 0; warmup < warmups; warmup++ {
			if err := ctx.Err(); err != nil {
				return samples, err
			}
			_, _ = engine.Extract(ctx, extract.Request{
				Schema:    string(c.Schema),
				MaxTokens: 2048,
				Delimiter: c.Case.Delimiter,
			}, []extract.Source{{Name: c.Case.ID, Reader: bytes.NewReader(c.Input)}})
		}

		for repetition := 0; repetition < repetitions; repetition++ {
			if err := ctx.Err(); err != nil {
				return samples, err
			}
			start := time.Now()
			result, err := engine.Extract(ctx, extract.Request{
				Schema:    string(c.Schema),
				MaxTokens: 2048,
				Delimiter: c.Case.Delimiter,
			}, []extract.Source{{Name: c.Case.ID, Reader: bytes.NewReader(c.Input)}})
			sample := Sample{
				CaseID:     c.Case.ID,
				Repetition: repetition,
				Metrics:    result.Metrics,
				WallTime:   time.Since(start),
			}
			if err != nil {
				sample.Score = ScoreCase(c, nil)
				sample.Error = err.Error()
			} else {
				sample.Score = ScoreCase(c, result.JSON)
				sample.Valid = sample.Score.ExactMatch
			}
			samples = append(samples, sample)
		}
	}
	return samples, nil
}

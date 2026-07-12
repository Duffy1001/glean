//go:build integration

package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/duffy1001/glean"
	"github.com/duffy1001/glean/llama"
)

func newIntegrationEngine(t *testing.T) *Engine {
	t.Helper()
	llama.SetLogLevel(6)
	modelPath, err := glean.ResolveModel("fast", false)
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	engine, err := NewEngine(context.Background(), Config{
		ModelPath:      modelPath,
		ContextSize:    8192,
		Threads:        4,
		GPULayers:      0,
		Device:         "cpu",
		GrammarEnabled: true,
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("close engine: %v", err)
		}
	})
	return engine
}

func extractString(t *testing.T, engine *Engine, schema, input string) Result {
	t.Helper()
	result, err := engine.Extract(context.Background(), Request{
		Schema:    schema,
		MaxTokens: 200,
		Delimiter: "\n",
	}, []Source{{Name: "test", Reader: strings.NewReader(input)}})
	if err != nil {
		t.Fatalf("extract %q: %v", input, err)
	}
	return result
}

func TestEngineReuseResetsContextAndGrammar(t *testing.T) {
	engine := newIntegrationEngine(t)
	arraySchema, err := BuildSchemaFromFields("name,age")
	if err != nil {
		t.Fatal(err)
	}
	objectSchema := `{
		"type": "object",
		"properties": {"status": {"type": "string"}},
		"required": ["status"],
		"additionalProperties": false
	}`

	firstA := extractString(t, engine, arraySchema, "Alice is 30 years old")
	_ = extractString(t, engine, objectSchema, "Status: healthy")
	secondA := extractString(t, engine, arraySchema, "Alice is 30 years old")

	if !bytes.Equal(firstA.JSON, secondA.JSON) {
		t.Fatalf("same request changed after an intervening request:\nfirst:  %s\nsecond: %s", firstA.JSON, secondA.JSON)
	}
}

func TestEngineCloseIsIdempotentAndPreventsExtraction(t *testing.T) {
	engine := newIntegrationEngine(t)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	schema, err := BuildSchemaFromFields("name")
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Extract(context.Background(), Request{
		Schema:    schema,
		MaxTokens: 200,
		Delimiter: "\n",
	}, []Source{{Name: "test", Reader: strings.NewReader("Alice")}})
	if err == nil {
		t.Fatal("extract after close should fail")
	}
}

func TestEngineRejectsCancelledContext(t *testing.T) {
	engine := newIntegrationEngine(t)
	schema, err := BuildSchemaFromFields("name")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = engine.Extract(ctx, Request{
		Schema:    schema,
		MaxTokens: 200,
		Delimiter: "\n",
	}, []Source{{Name: "test", Reader: strings.NewReader("Alice")}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestEngineReportsStructuredMetrics(t *testing.T) {
	engine := newIntegrationEngine(t)
	schema, err := BuildSchemaFromFields("name,age")
	if err != nil {
		t.Fatal(err)
	}
	result := extractString(t, engine, schema, "Alice is 30 years old")
	metrics := result.Metrics
	if metrics.InputBytes == 0 || metrics.OutputBytes == 0 {
		t.Fatalf("missing byte metrics: %#v", metrics)
	}
	if metrics.PromptTokens == 0 || metrics.GeneratedTokens == 0 {
		t.Fatalf("missing token metrics: %#v", metrics)
	}
	if metrics.RecordsProduced == 0 || metrics.ChunksProcessed != 1 {
		t.Fatalf("unexpected record or chunk metrics: %#v", metrics)
	}
	if metrics.TotalTime <= 0 || metrics.TokenizeTime <= 0 || metrics.PrefillTime <= 0 || metrics.GenerationTime <= 0 {
		t.Fatalf("missing stage timing: %#v", metrics)
	}
	if len(result.ChunkRuns) != 1 {
		t.Fatalf("chunk runs = %d, want 1", len(result.ChunkRuns))
	}
	if result.ChunkRuns[0].GeneratedTokens != metrics.GeneratedTokens {
		t.Fatalf("chunk and aggregate generated tokens differ: %#v / %#v", result.ChunkRuns[0], metrics)
	}
	if _, err := json.Marshal(result); err != nil {
		t.Fatalf("metrics result should be JSON serializable: %v", err)
	}
}

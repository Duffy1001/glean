//go:build integration

package extract

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/duffy1001/glean"
)

func newIntegrationEngine(t *testing.T) *Engine {
	t.Helper()
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

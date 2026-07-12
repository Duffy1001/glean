package extract

import (
	"bytes"
	"context"
	"testing"
)

func BenchmarkBuildSchemaFromFields(b *testing.B) {
	fields := "name,age,email,active,timestamp,service"
	b.ReportAllocs()
	for b.Loop() {
		if _, err := BuildSchemaFromFields(fields); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeDelimiter(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := DecodeDelimiter(`\n\t`); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSplitDelimiter(b *testing.B) {
	splitFn := splitDelimiter([]byte("||"))
	data := []byte("one||two||three||four")
	b.ReportAllocs()
	var advance int
	var token []byte
	for b.Loop() {
		advance, token, _ = splitFn(data, false)
	}
	_ = advance
	_ = token
}

func BenchmarkStreamReaderChunks(b *testing.B) {
	ctx := context.Background()
	delimiter := "\n"
	input := bytes.Repeat([]byte("one\ntwo\n"), 1000)
	byteBudget := 24 * 1024
	yield := func(string) error { return nil }
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for b.Loop() {
		r := bytes.NewReader(input)
		if _, err := streamReaderChunks(ctx, r, byteBudget, delimiter, yield); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompileSchema(b *testing.B) {
	schema := `{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"string"}},"required":["name","age"],"additionalProperties":false}}`
	b.ReportAllocs()
	for b.Loop() {
		if _, err := newSchemaValidator(schema); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateJSON(b *testing.B) {
	schema := `{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"string"}},"required":["name","age"],"additionalProperties":false}}`
	v, err := newSchemaValidator(schema)
	if err != nil {
		b.Fatal(err)
	}
	jsonStr := `[{"name":"Alice","age":"30"}]`
	b.ReportAllocs()
	for b.Loop() {
		if err := v.validate(jsonStr); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSchemaToGrammar(b *testing.B) {
	schema := `{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}}`
	// This isn't a hot-path benchmark; avoid failing due to one-off cache issues.
	if _, err := jsonSchemaToGBNF(schema); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := jsonSchemaToGBNF(schema); err != nil {
			b.Fatal(err)
		}
	}
}

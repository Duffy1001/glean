package eval

import "testing"

func TestScoreCaseExactIdentityMatch(t *testing.T) {
	c := LoadedCase{
		Case:     Case{ID: "people", Tier: "conformance", Comparison: "ordered", IdentityFields: []string{"id"}},
		Schema:   []byte(`{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"}},"required":["id","name"],"additionalProperties":false}}`),
		Expected: []byte(`[{"id":"a","name":"Alice"},{"id":"b","name":"Bob"}]`),
	}
	score := ScoreCase(c, []byte(`[{"id":"a","name":"Alice"},{"id":"b","name":"Bob"}]`))
	if !score.ExactMatch || !score.OrderCorrect || score.MatchedRecords != 2 {
		t.Fatalf("unexpected exact score: %#v", score)
	}
}

func TestScoreCaseReportsOrderExtraAndTypedDifferences(t *testing.T) {
	c := LoadedCase{
		Case:     Case{ID: "people", Tier: "conformance", Comparison: "ordered", IdentityFields: []string{"id"}},
		Schema:   []byte(`{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"age":{"type":"integer"}},"required":["id","age"],"additionalProperties":false}}`),
		Expected: []byte(`[{"id":"a","age":30},{"id":"b","age":25}]`),
	}
	score := ScoreCase(c, []byte(`[{"id":"b","age":25},{"id":"a","age":"30"},{"id":"c","age":40}]`))
	if score.ExactMatch || score.OrderCorrect {
		t.Fatalf("reordered and mismatched output scored exact: %#v", score)
	}
	if score.ExtraRecords != 1 || score.MissingRecords != 0 {
		t.Fatalf("unexpected record counts: %#v", score)
	}
	if score.IncorrectFields == 0 || score.SchemaValid {
		t.Fatalf("typed mismatch was not surfaced: %#v", score)
	}
}

func TestScoreCaseReportsMissingFields(t *testing.T) {
	c := LoadedCase{
		Case:     Case{ID: "object", Tier: "conformance", Comparison: "positional"},
		Schema:   []byte(`{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"integer"}},"required":["name","age"],"additionalProperties":false}`),
		Expected: []byte(`{"name":"Alice","age":30}`),
	}
	score := ScoreCase(c, []byte(`{"name":"Alice"}`))
	if score.MissingFields != 1 || score.SchemaValid || score.ExactMatch {
		t.Fatalf("missing field was not surfaced: %#v", score)
	}
}

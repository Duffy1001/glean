package eval

type QualitySummary struct {
	TotalCases            int     `json:"total_cases"`
	PassedCases           int     `json:"passed_cases"`
	JSONValidRate         float64 `json:"json_valid_rate"`
	SchemaValidRate       float64 `json:"schema_valid_rate"`
	RecordRecall          float64 `json:"record_recall"`
	RecordPrecision       float64 `json:"record_precision"`
	FieldAccuracy         float64 `json:"field_accuracy"`
	OrderPreservationRate float64 `json:"order_preservation_rate"`
	UnexpectedFieldRate   float64 `json:"unexpected_field_rate"`
}

func Summarize(samples []Sample) QualitySummary {
	var summary QualitySummary
	var jsonValid, schemaValid, matched, expected, actual, correctFields, expectedFields, orderCorrect, unexpectedFields int
	for _, sample := range samples {
		score := sample.Score
		summary.TotalCases++
		if score.ExactMatch {
			summary.PassedCases++
		}
		if score.JSONValid {
			jsonValid++
		}
		if score.SchemaValid {
			schemaValid++
		}
		matched += score.MatchedRecords
		expected += score.ExpectedRecords
		actual += score.ActualRecords
		correctFields += score.CorrectFields
		expectedFields += score.ExpectedFields
		if score.OrderCorrect {
			orderCorrect++
		}
		unexpectedFields += score.UnexpectedFields
	}
	summary.JSONValidRate = ratio(jsonValid, summary.TotalCases)
	summary.SchemaValidRate = ratio(schemaValid, summary.TotalCases)
	summary.RecordRecall = ratio(matched, expected)
	summary.RecordPrecision = ratio(matched, actual)
	summary.FieldAccuracy = ratio(correctFields, expectedFields)
	summary.OrderPreservationRate = ratio(orderCorrect, summary.TotalCases)
	summary.UnexpectedFieldRate = ratio(unexpectedFields, expectedFields)
	return summary
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

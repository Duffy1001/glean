package eval

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type Difference struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

type CaseScore struct {
	CaseID string `json:"case_id"`
	Tier   string `json:"tier"`

	JSONValid   bool `json:"json_valid"`
	SchemaValid bool `json:"schema_valid"`

	ExpectedRecords int `json:"expected_records"`
	ActualRecords   int `json:"actual_records"`
	MatchedRecords  int `json:"matched_records"`
	MissingRecords  int `json:"missing_records"`
	ExtraRecords    int `json:"extra_records"`

	ExpectedFields   int `json:"expected_fields"`
	CorrectFields    int `json:"correct_fields"`
	IncorrectFields  int `json:"incorrect_fields"`
	MissingFields    int `json:"missing_fields"`
	UnexpectedFields int `json:"unexpected_fields"`

	OrderCorrect bool         `json:"order_correct"`
	ExactMatch   bool         `json:"exact_match"`
	Differences  []Difference `json:"differences"`
}

func ScoreCase(c LoadedCase, actual []byte) CaseScore {
	score := CaseScore{CaseID: c.Case.ID, Tier: c.Case.Tier, Differences: make([]Difference, 0)}
	var expectedValue any
	if err := json.Unmarshal(c.Expected, &expectedValue); err != nil {
		score.addDifference("$", "invalid_expected", err.Error(), "")
		return score
	}
	var actualValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		score.addDifference("$", "invalid_json", "valid JSON", string(actual))
		return score
	}
	score.JSONValid = true
	if err := validateExpected(c.Schema, actual); err == nil {
		score.SchemaValid = true
	} else {
		score.addDifference("$", "schema_validation", "schema-valid JSON", err.Error())
	}

	expectedArray, expectedIsArray := expectedValue.([]any)
	actualArray, actualIsArray := actualValue.([]any)
	if expectedIsArray || actualIsArray {
		if !expectedIsArray || !actualIsArray {
			score.addDifference("$", "type", formatValue(expectedValue), formatValue(actualValue))
			return score.finish()
		}
		if len(c.Case.IdentityFields) > 0 {
			score.scoreIdentityArray(c, expectedArray, actualArray)
		} else {
			score.scorePositionalArray(c, expectedArray, actualArray)
		}
		return score.finish()
	}

	score.ExpectedRecords = 1
	score.ActualRecords = 1
	score.MatchedRecords = 1
	score.OrderCorrect = true
	score.compareValue("$", expectedValue, actualValue, c.Case.AllowAdditionalFields)
	return score.finish()
}

func (s *CaseScore) scoreIdentityArray(c LoadedCase, expected, actual []any) {
	s.ExpectedRecords = len(expected)
	s.ActualRecords = len(actual)
	expectedByID, expectedOrder, expectedOK := recordsByIdentity(expected, c.Case.IdentityFields)
	actualByID, actualOrder, actualOK := recordsByIdentity(actual, c.Case.IdentityFields)
	if !expectedOK || !actualOK {
		s.addDifference("$", "identity", "records with unique identity fields", "missing or duplicate identity")
		s.scorePositionalArray(c, expected, actual)
		return
	}

	s.OrderCorrect = reflect.DeepEqual(expectedOrder, actualOrder)
	if !s.OrderCorrect {
		s.addDifference("$", "order", formatValue(expectedOrder), formatValue(actualOrder))
	}
	for _, id := range expectedOrder {
		expectedRecord := expectedByID[id]
		actualRecord, ok := actualByID[id]
		if !ok {
			s.MissingRecords++
			s.addDifference("$["+id+"]", "missing_record", formatValue(expectedRecord), "")
			continue
		}
		s.MatchedRecords++
		s.compareValue("$["+id+"]", expectedRecord, actualRecord, c.Case.AllowAdditionalFields)
	}
	for _, id := range actualOrder {
		if _, ok := expectedByID[id]; !ok {
			s.ExtraRecords++
			s.addDifference("$["+id+"]", "extra_record", "", formatValue(actualByID[id]))
		}
	}
}

func (s *CaseScore) scorePositionalArray(c LoadedCase, expected, actual []any) {
	s.ExpectedRecords = len(expected)
	s.ActualRecords = len(actual)
	limit := len(expected)
	if len(actual) < limit {
		limit = len(actual)
	}
	for i := 0; i < limit; i++ {
		s.MatchedRecords++
		s.compareValue(fmt.Sprintf("$[%d]", i), expected[i], actual[i], c.Case.AllowAdditionalFields)
	}
	for i := limit; i < len(expected); i++ {
		s.MissingRecords++
		s.addDifference(fmt.Sprintf("$[%d]", i), "missing_record", formatValue(expected[i]), "")
	}
	for i := limit; i < len(actual); i++ {
		s.ExtraRecords++
		s.addDifference(fmt.Sprintf("$[%d]", i), "extra_record", "", formatValue(actual[i]))
	}
	s.OrderCorrect = s.MissingRecords == 0 && s.ExtraRecords == 0 && len(s.Differences) == 0
}

func (s *CaseScore) compareValue(path string, expected, actual any, allowAdditional bool) {
	expectedObject, expectedIsObject := expected.(map[string]any)
	actualObject, actualIsObject := actual.(map[string]any)
	if expectedIsObject || actualIsObject {
		if !expectedIsObject || !actualIsObject {
			s.IncorrectFields++
			s.addDifference(path, "type", formatValue(expected), formatValue(actual))
			return
		}
		for key, expectedValue := range expectedObject {
			s.ExpectedFields++
			actualValue, ok := actualObject[key]
			if !ok {
				s.MissingFields++
				s.addDifference(path+"."+key, "missing_field", formatValue(expectedValue), "")
				continue
			}
			before := len(s.Differences)
			s.compareValue(path+"."+key, expectedValue, actualValue, allowAdditional)
			if len(s.Differences) == before {
				s.CorrectFields++
			}
		}
		if !allowAdditional {
			for key, actualValue := range actualObject {
				if _, ok := expectedObject[key]; !ok {
					s.UnexpectedFields++
					s.addDifference(path+"."+key, "unexpected_field", "", formatValue(actualValue))
				}
			}
		}
		return
	}

	expectedArray, expectedIsArray := expected.([]any)
	actualArray, actualIsArray := actual.([]any)
	if expectedIsArray || actualIsArray {
		if !expectedIsArray || !actualIsArray || !reflect.DeepEqual(expectedArray, actualArray) {
			s.IncorrectFields++
			s.addDifference(path, "value", formatValue(expected), formatValue(actual))
		}
		return
	}
	if !reflect.DeepEqual(expected, actual) {
		s.IncorrectFields++
		s.addDifference(path, "value", formatValue(expected), formatValue(actual))
	}
}

func (s *CaseScore) finish() CaseScore {
	s.ExactMatch = s.JSONValid && s.SchemaValid && s.MissingRecords == 0 && s.ExtraRecords == 0 && s.IncorrectFields == 0 && s.MissingFields == 0 && s.UnexpectedFields == 0 && s.OrderCorrect
	return *s
}

func (s *CaseScore) addDifference(path, kind, expected, actual string) {
	s.Differences = append(s.Differences, Difference{Path: path, Kind: kind, Expected: expected, Actual: actual})
}

func recordsByIdentity(records []any, fields []string) (map[string]any, []string, bool) {
	byID := make(map[string]any, len(records))
	order := make([]string, 0, len(records))
	for _, record := range records {
		object, ok := record.(map[string]any)
		if !ok {
			return nil, nil, false
		}
		values := make([]any, len(fields))
		for i, field := range fields {
			value, ok := object[field]
			if !ok {
				return nil, nil, false
			}
			values[i] = value
		}
		id := formatValue(values)
		if _, duplicate := byID[id]; duplicate {
			return nil, nil, false
		}
		byID[id] = record
		order = append(order, id)
	}
	return byID, order, true
}

func formatValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

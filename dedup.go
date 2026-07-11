package main

import (
	"fmt"
)

// dedupByPK merges records with the same primary key value.
// Later occurrences overwrite earlier ones for conflicting fields.
// Fields present in only one record are preserved (union merge).
// Order of first occurrence is maintained.
func dedupByPK(records []interface{}, pk string) []interface{} {
	seen := make(map[string]int)
	var result []interface{}

	for _, rec := range records {
		m, ok := rec.(map[string]interface{})
		if !ok {
			result = append(result, rec)
			continue
		}

		key := fmt.Sprintf("%v", m[pk])
		if idx, found := seen[key]; found {
			existing := result[idx].(map[string]interface{})
			for k, v := range m {
				existing[k] = v
			}
		} else {
			seen[key] = len(result)
			result = append(result, m)
		}
	}

	return result
}

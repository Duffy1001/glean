package app

import (
	"encoding/json"
	"fmt"
	"io"
)

func writeJSON(w io.Writer, raw json.RawMessage, compact bool) error {
	var parsed interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("invalid JSON output: %w", err)
	}
	var out []byte
	var err error
	if compact {
		out, err = json.Marshal(parsed)
	} else {
		out, err = json.MarshalIndent(parsed, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("marshal JSON output: %w", err)
	}
	_, err = fmt.Fprintln(w, string(out))
	return err
}

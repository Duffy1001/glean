package eval

import "encoding/json"

type Case struct {
	ID                    string         `json:"id"`
	Description           string         `json:"description"`
	Tier                  string         `json:"tier"`
	Tags                  []string       `json:"tags"`
	Delimiter             string         `json:"delimiter"`
	IdentityFields        []string       `json:"identity_fields"`
	Comparison            string         `json:"comparison"`
	AllowAdditionalFields bool           `json:"allow_additional_fields"`
	Normalizers           map[string]any `json:"normalizers"`
}

type LoadedCase struct {
	Case     Case
	CaseDir  string
	Input    []byte
	Schema   json.RawMessage
	Expected json.RawMessage
}

type Manifest struct {
	Version int      `json:"version"`
	Model   string   `json:"model"`
	Cases   []string `json:"cases"`
}

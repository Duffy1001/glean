package eval

import "time"

type Report struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Mode        string         `json:"mode"`
	Corpus      Manifest       `json:"corpus"`
	Samples     []Sample       `json:"samples"`
	Quality     QualitySummary `json:"quality"`
}

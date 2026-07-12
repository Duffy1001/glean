package extract

import "encoding/json"

type Result struct {
	JSON      json.RawMessage `json:"json"`
	Metrics   Metrics         `json:"metrics"`
	ChunkRuns []ChunkMetrics  `json:"chunk_runs"`
}

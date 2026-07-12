package eval

import (
	"time"

	"github.com/duffy1001/glean/internal/benchmeta"
)

type Report struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Mode        string             `json:"mode"`
	Corpus      Manifest           `json:"corpus"`
	Metadata    benchmeta.Metadata `json:"metadata"`
	Samples     []Sample           `json:"samples"`
	Quality     QualitySummary     `json:"quality"`
}

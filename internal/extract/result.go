package extract

import (
	"encoding/json"
	"time"
)

type Result struct {
	JSON            json.RawMessage
	GeneratedTokens int
	TotalTime       time.Duration
}

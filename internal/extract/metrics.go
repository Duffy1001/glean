package extract

import "time"

type Metrics struct {
	InputBytes        int64         `json:"input_bytes"`
	OutputBytes       int64         `json:"output_bytes"`
	PromptTokens      int           `json:"prompt_tokens"`
	GeneratedTokens   int           `json:"generated_tokens"`
	RecordsProduced   int           `json:"records_produced"`
	ChunksProcessed   int           `json:"chunks_processed"`
	PromptBuildTime   time.Duration `json:"prompt_build_time_ns"`
	TokenizeTime      time.Duration `json:"tokenize_time_ns"`
	PrefillTime       time.Duration `json:"prefill_time_ns"`
	TimeToFirstToken  time.Duration `json:"time_to_first_token_ns"`
	GenerationTime    time.Duration `json:"generation_time_ns"`
	ParseTime         time.Duration `json:"parse_time_ns"`
	ValidationTime    time.Duration `json:"validation_time_ns"`
	SerializationTime time.Duration `json:"serialization_time_ns"`
	TotalTime         time.Duration `json:"total_time_ns"`
}

type ChunkMetrics struct {
	Index            int           `json:"index"`
	InputBytes       int64         `json:"input_bytes"`
	PromptTokens     int           `json:"prompt_tokens"`
	GeneratedTokens  int           `json:"generated_tokens"`
	RecordsProduced  int           `json:"records_produced"`
	Retried          bool          `json:"retried"`
	RetryDepth       int           `json:"retry_depth"`
	PromptBuildTime  time.Duration `json:"prompt_build_time_ns"`
	TokenizeTime     time.Duration `json:"tokenize_time_ns"`
	PrefillTime      time.Duration `json:"prefill_time_ns"`
	TimeToFirstToken time.Duration `json:"time_to_first_token_ns"`
	GenerationTime   time.Duration `json:"generation_time_ns"`
	ParseTime        time.Duration `json:"parse_time_ns"`
	ValidationTime   time.Duration `json:"validation_time_ns"`
	TotalTime        time.Duration `json:"total_time_ns"`
}

func (m *Metrics) addChunk(chunk ChunkMetrics) {
	m.InputBytes += chunk.InputBytes
	m.PromptTokens += chunk.PromptTokens
	m.GeneratedTokens += chunk.GeneratedTokens
	m.RecordsProduced += chunk.RecordsProduced
	m.ChunksProcessed++
	m.PromptBuildTime += chunk.PromptBuildTime
	m.TokenizeTime += chunk.TokenizeTime
	m.PrefillTime += chunk.PrefillTime
	m.TimeToFirstToken += chunk.TimeToFirstToken
	m.GenerationTime += chunk.GenerationTime
	m.ParseTime += chunk.ParseTime
	m.ValidationTime += chunk.ValidationTime
}

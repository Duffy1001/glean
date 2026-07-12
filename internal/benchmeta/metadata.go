package benchmeta

import (
	"runtime"
	"time"

	"github.com/duffy1001/glean"
)

// Metadata captures the machine + build context for evaluator runs.
//
// Some fields may be set to "unknown" when optional data cannot be discovered
// without adding platform-specific dependencies.
type Metadata struct {
	Timestamp      time.Time `json:"timestamp"`
	GleanCommit    string    `json:"glean_commit"`
	LlamaCommit    string    `json:"llama_commit"`
	ModelName      string    `json:"model_name"`
	ModelSHA256    string    `json:"model_sha256"`
	GoVersion      string    `json:"go_version"`
	OS             string    `json:"os"`
	Architecture   string    `json:"architecture"`
	LogicalCPUs    int       `json:"logical_cpus"`
	Threads        int       `json:"threads"`
	ContextSize    int       `json:"context_size"`
	GPULayers      int       `json:"gpu_layers"`
	GrammarEnabled bool      `json:"grammar_enabled"`
	BuildVariant   string    `json:"build_variant"`
}

func Collect(timestamp time.Time, modelName string, threads, contextSize, gpuLayers int, grammarEnabled bool) Metadata {
	return Metadata{
		Timestamp:      timestamp,
		GleanCommit:    glean.Version,
		LlamaCommit:    "unknown",
		ModelName:      modelName,
		ModelSHA256:    "unknown",
		GoVersion:      runtime.Version(),
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		LogicalCPUs:    runtime.NumCPU(),
		Threads:        threads,
		ContextSize:    contextSize,
		GPULayers:      gpuLayers,
		GrammarEnabled: grammarEnabled,
		BuildVariant:   glean.BuildVariant(),
	}
}

package extract

type Config struct {
	ModelPath        string
	ContextSize      int
	Threads          int
	GPULayers        int
	Device           string
	AllowCPUFallback bool
	GrammarEnabled   bool
	Verbose          bool
}

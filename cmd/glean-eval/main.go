package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/duffy1001/glean"
	"github.com/duffy1001/glean/internal/eval"
	"github.com/duffy1001/glean/internal/extract"
)

func main() {
	corpusPath := flag.String("corpus", "benchdata/corpus-v1", "Corpus directory")
	modelPath := flag.String("model", "", "GGUF model path; resolves fast when empty")
	mode := flag.String("mode", "quality", "Evaluation mode: quality")
	threads := flag.Int("threads", 4, "CPU inference threads")
	contextSize := flag.Int("ctx", 8192, "Context window size")
	device := flag.String("device", "cpu", "Inference device: cpu or auto")
	gpuLayers := flag.Int("gpu-layers", 0, "Model layers to offload")
	noGrammar := flag.Bool("no-grammar", false, "Disable grammar-constrained generation")
	repetitions := flag.Int("repetitions", 1, "Runs per corpus case")
	output := flag.String("output", "", "Write JSON report to this file instead of stdout")
	flag.Parse()

	if *mode != "quality" {
		fmt.Fprintln(os.Stderr, "only --mode quality is available")
		os.Exit(1)
	}
	if *device != "cpu" && *device != "auto" {
		fmt.Fprintln(os.Stderr, "--device must be cpu or auto")
		os.Exit(1)
	}
	if *modelPath == "" {
		path, err := glean.ResolveModel("fast", false)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		*modelPath = path
	}

	corpus, err := eval.LoadCorpus(*corpusPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer extract.ShutdownBackend()
	engine, err := extract.NewEngine(context.Background(), extract.Config{
		ModelPath:        *modelPath,
		ContextSize:      *contextSize,
		Threads:          *threads,
		GPULayers:        *gpuLayers,
		Device:           *device,
		AllowCPUFallback: *device == "auto",
		GrammarEnabled:   !*noGrammar,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer engine.Close()

	samples, err := eval.RunQuality(context.Background(), corpus, engine, *repetitions)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	report := eval.Report{
		GeneratedAt: time.Now().UTC(),
		Mode:        *mode,
		Corpus:      corpus.Manifest,
		Samples:     samples,
		Quality:     eval.Summarize(samples),
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *output == "" {
		fmt.Println(string(data))
		return
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

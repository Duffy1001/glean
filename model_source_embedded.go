//go:build embedded

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"

	"github.com/klauspost/compress/zstd"
)

//go:embed assets/qwen3-0.6b-q4_k_m.gguf.zst
var embeddedFastModel []byte

func materializeModel(info ModelInfo, dest string, verbose bool) (string, error) {
	if info.Filename != modelRegistry["fast"].Filename {
		return downloadModel(info, dest, verbose)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Extracting embedded %s...\n", info.Name)
	}

	decoder, err := zstd.NewReader(bytes.NewReader(embeddedFastModel))
	if err != nil {
		return "", fmt.Errorf("open embedded model: %w", err)
	}
	defer decoder.Close()
	if _, err := installModel(dest, info, decoder); err != nil {
		return "", fmt.Errorf("extract embedded model: %w", err)
	}
	return dest, nil
}

func buildEdition() string {
	return "full"
}

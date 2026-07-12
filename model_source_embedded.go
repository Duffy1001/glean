//go:build embedded

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"

	"github.com/klauspost/compress/zstd"
)

func materializeModel(info ModelInfo, dest string, verbose bool) (string, error) {
	if info.Filename != embeddedModelFilename() {
		return downloadModel(info, dest, verbose)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Extracting embedded %s...\n", info.Name)
	}

	decoder, err := zstd.NewReader(bytes.NewReader(embeddedModel))
	if err != nil {
		return "", fmt.Errorf("open embedded model: %w", err)
	}
	defer decoder.Close()
	if _, err := installModel(dest, info, decoder); err != nil {
		return "", fmt.Errorf("extract embedded model: %w", err)
	}
	return dest, nil
}

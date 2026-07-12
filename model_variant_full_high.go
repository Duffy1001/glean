//go:build embedded && high

package main

import _ "embed"

//go:embed assets/qwen3-1.7b-q4_k_m.gguf.zst
var embeddedModel []byte

func embeddedModelFilename() string {
	return modelRegistry["quality"].Filename
}

func defaultModel() string {
	return "quality"
}

func buildVariant() string {
	return "full-high"
}

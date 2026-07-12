//go:build embedded

package main

import _ "embed"

//go:embed assets/qwen3-0.6b-q4_k_m.gguf.zst
var embeddedModel []byte

func defaultModel() string {
	return "fast"
}

func buildVariant() string {
	return "full-fast"
}

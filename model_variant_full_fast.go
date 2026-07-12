//go:build embedded

package glean

import _ "embed"

//go:embed assets/qwen3-0.6b-q4_k_m.gguf.zst
var embeddedModel []byte

func DefaultModel() string {
	return "fast"
}

func BuildVariant() string {
	return "full-fast"
}

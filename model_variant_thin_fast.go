//go:build !embedded && !high

package main

func defaultModel() string {
	return "fast"
}

func buildVariant() string {
	return "thin-fast"
}

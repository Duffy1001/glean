//go:build !embedded && high

package main

func defaultModel() string {
	return "quality"
}

func buildVariant() string {
	return "thin-high"
}

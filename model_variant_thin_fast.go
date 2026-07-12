//go:build !embedded

package main

func defaultModel() string {
	return "fast"
}

func buildVariant() string {
	return "thin-fast"
}

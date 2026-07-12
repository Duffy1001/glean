//go:build !embedded

package main

func materializeModel(info ModelInfo, dest string, verbose bool) (string, error) {
	return downloadModel(info, dest, verbose)
}

func buildEdition() string {
	return "thin"
}

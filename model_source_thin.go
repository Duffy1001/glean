//go:build !embedded

package glean

func materializeModel(info modelInfo, dest string, verbose bool) (string, error) {
	return downloadModel(info, dest, verbose)
}

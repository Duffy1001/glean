//go:build !embedded

package glean

func DefaultModel() string {
	return "fast"
}

func BuildVariant() string {
	return "thin-fast"
}

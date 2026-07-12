package app

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/duffy1001/glean/internal/extract"
)

func openSources(paths []string, stdin io.Reader) ([]extract.Source, func() error, error) {
	if len(paths) == 0 {
		return []extract.Source{{Name: "stdin", Reader: stdin}}, func() error { return nil }, nil
	}

	sources := make([]extract.Source, 0, len(paths))
	closers := make([]io.Closer, 0, len(paths))
	closeAll := func() error {
		var errs []error
		for i := len(closers) - 1; i >= 0; i-- {
			if err := closers[i].Close(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}

	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			_ = closeAll()
			return nil, nil, fmt.Errorf("open %s: %w", path, err)
		}
		closers = append(closers, f)
		sources = append(sources, extract.Source{Name: path, Reader: f})
	}
	return sources, closeAll, nil
}

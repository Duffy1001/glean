package extract

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

func streamReaderChunks(ctx context.Context, r io.Reader, byteBudget int, delimiter string, yield func(string) error) (bool, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	scanner.Split(splitDelimiter([]byte(delimiter)))
	var records []string
	currentBytes := 0
	hadInput := false

	emit := func() error {
		if len(records) == 0 {
			return nil
		}
		chunk := strings.Join(records, delimiter)
		if strings.TrimSpace(chunk) != "" {
			hadInput = true
			if err := yield(chunk); err != nil {
				return err
			}
		}
		records = nil
		currentBytes = 0
		return nil
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return hadInput, err
		}
		record := scanner.Text()
		if record != "" {
			if len(records) > 0 && currentBytes+len(record) > byteBudget {
				if emitErr := emit(); emitErr != nil {
					return hadInput, emitErr
				}
			}
			records = append(records, record)
			currentBytes += len(record)
		}
	}
	if err := scanner.Err(); err != nil {
		return hadInput, err
	}
	if err := emit(); err != nil {
		return hadInput, err
	}
	return hadInput, nil
}

func streamSources(ctx context.Context, sources []Source, byteBudget int, delimiter string, yield func(string) error) (bool, error) {
	hadInput := false
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return hadInput, err
		}
		hadFileInput, readErr := streamReaderChunks(ctx, src.Reader, byteBudget, delimiter, yield)
		hadInput = hadInput || hadFileInput
		if readErr != nil {
			return hadInput, fmt.Errorf("read %s: %w", src.Name, readErr)
		}
	}
	return hadInput, nil
}

func splitDelimiter(delimiter []byte) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if index := bytes.Index(data, delimiter); index >= 0 {
			return index + len(delimiter), data[:index], nil
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
}

func DecodeDelimiter(value string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			continue
		}
		if i+1 >= len(value) {
			return "", errors.New("trailing escape")
		}
		i++
		switch value[i] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case '0':
			out.WriteByte(0)
		case 'r':
			out.WriteByte('\r')
		case '\\':
			out.WriteByte('\\')
		default:
			return "", fmt.Errorf("unsupported escape \\%c", value[i])
		}
	}
	if out.Len() == 0 {
		return "", errors.New("delimiter cannot be empty")
	}
	return out.String(), nil
}

func readSources(sources []Source) (string, error) {
	var input strings.Builder
	for _, src := range sources {
		data, err := io.ReadAll(src.Reader)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", src.Name, err)
		}
		input.Write(data)
		input.WriteByte('\n')
	}
	return input.String(), nil
}

func splitChunk(chunk, delimiter string) (string, string, bool) {
	parts := strings.Split(chunk, delimiter)
	if len(parts) > 1 {
		mid := len(parts) / 2
		if mid > 0 && mid < len(parts) {
			return strings.Join(parts[:mid], delimiter), strings.Join(parts[mid:], delimiter), true
		}
	}

	runes := []rune(chunk)
	if len(runes) < 2 {
		return "", "", false
	}
	mid := len(runes) / 2
	return string(runes[:mid]), string(runes[mid:]), true
}

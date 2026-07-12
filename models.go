package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

type ModelInfo struct {
	Name     string
	Filename string
	URL      string
	SHA256   string
	Size     int64
}

var modelRegistry = map[string]ModelInfo{
	"fast": {
		Name:     "qwen3-0.6b",
		Filename: "qwen3-0.6b-q4_k_m.gguf",
		URL:      "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q4_K_M.gguf",
		SHA256:   "ac2d97712095a558e31573f62f466a3f9d93990898b0ec79d7c974c1780d524a",
		Size:     396705472,
	},
	"quality": {
		Name:     "qwen3-1.7b",
		Filename: "qwen3-1.7b-q4_k_m.gguf",
		URL:      "https://huggingface.co/unsloth/Qwen3-1.7B-GGUF/resolve/main/Qwen3-1.7B-Q4_K_M.gguf",
		SHA256:   "b139949c5bd74937ad8ed8c8cf3d9ffb1e99c866c823204dc42c0d91fa181897",
		Size:     1107409472,
	},
}

func modelCacheDir() (string, error) {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "glean", "models"), nil
	}
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Caches", "glean", "models"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "glean", "models"), nil
}

func resolveModel(choice string, verbose bool) (string, error) {
	info, ok := modelRegistry[choice]
	if !ok {
		return "", fmt.Errorf("unknown model %q (available: fast, quality)", choice)
	}

	cacheDir, err := modelCacheDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(cacheDir, info.Filename)
	valid, err := verifyModel(path, info)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("verify cached model: %w", err)
	}
	if valid {
		return path, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove invalid cached model: %w", err)
	}

	return materializeModel(info, path, verbose)
}

func downloadModel(info ModelInfo, dest string, verbose bool) (string, error) {
	if verbose {
		fmt.Fprintf(os.Stderr, "Downloading %s (%s)...\n", info.Name, humanSize(info.Size))
	}

	resp, err := http.Get(info.URL)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	n, err := installModel(dest, info, resp.Body)
	if err != nil {
		return "", fmt.Errorf("download model: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Downloaded %s (%s, %d bytes)\n", info.Name, humanSize(n), n)
	}
	return dest, nil
}

func verifyModel(path string, info ModelInfo) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return false, err
	}
	if stat.Size() != info.Size {
		return false, nil
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == info.SHA256, nil
}

func installModel(dest string, info ModelInfo, src io.Reader) (int64, error) {
	tmp, err := os.CreateTemp(filepath.Dir(dest), info.Filename+".tmp-*")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), src)
	closeErr := tmp.Close()
	if copyErr != nil {
		return n, copyErr
	}
	if closeErr != nil {
		return n, closeErr
	}
	if n != info.Size {
		return n, fmt.Errorf("size mismatch: got %d, want %d", n, info.Size)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != info.SHA256 {
		return n, fmt.Errorf("checksum mismatch: got %s, want %s", got, info.SHA256)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		if valid, verifyErr := verifyModel(dest, info); verifyErr == nil && valid {
			return n, nil
		}
		return n, err
	}
	return n, nil
}

func humanSize(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAndVerifyModel(t *testing.T) {
	content := "model data"
	sum := sha256.Sum256([]byte(content))
	info := ModelInfo{
		Filename: "model.gguf",
		SHA256:   hex.EncodeToString(sum[:]),
		Size:     int64(len(content)),
	}
	dest := filepath.Join(t.TempDir(), info.Filename)

	if _, err := installModel(dest, info, strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	valid, err := verifyModel(dest, info)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("installed model did not verify")
	}

	if err := os.WriteFile(dest, []byte("corrupted!"), 0o644); err != nil {
		t.Fatal(err)
	}
	valid, err = verifyModel(dest, info)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("corrupt model verified successfully")
	}
}

func TestInstallModelRejectsChecksumMismatch(t *testing.T) {
	info := ModelInfo{Filename: "model.gguf", SHA256: strings.Repeat("0", 64), Size: 4}
	dest := filepath.Join(t.TempDir(), info.Filename)
	if _, err := installModel(dest, info, strings.NewReader("data")); err == nil {
		t.Fatal("expected checksum error")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("invalid model was installed: %v", err)
	}
}

package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Corpus struct {
	Root     string
	Manifest Manifest
	Cases    []LoadedCase
}

func LoadCorpus(root string) (Corpus, error) {
	manifestData, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return Corpus{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return Corpus{}, fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.Version < 1 {
		return Corpus{}, fmt.Errorf("unsupported corpus version %d", manifest.Version)
	}
	if len(manifest.Cases) == 0 {
		return Corpus{}, fmt.Errorf("manifest has no cases")
	}

	corpus := Corpus{Root: root, Manifest: manifest, Cases: make([]LoadedCase, 0, len(manifest.Cases))}
	seen := make(map[string]struct{}, len(manifest.Cases))
	for _, id := range manifest.Cases {
		if id == "" || filepath.Base(id) != id || strings.Contains(id, "..") {
			return Corpus{}, fmt.Errorf("invalid case id %q", id)
		}
		if _, ok := seen[id]; ok {
			return Corpus{}, fmt.Errorf("duplicate case id %q", id)
		}
		seen[id] = struct{}{}

		loaded, err := loadCase(filepath.Join(root, "cases", id))
		if err != nil {
			return Corpus{}, fmt.Errorf("load case %s: %w", id, err)
		}
		if loaded.Case.ID != id {
			return Corpus{}, fmt.Errorf("case id mismatch: manifest %q, case %q", id, loaded.Case.ID)
		}
		corpus.Cases = append(corpus.Cases, loaded)
	}
	return corpus, nil
}

func loadCase(dir string) (LoadedCase, error) {
	caseData, err := os.ReadFile(filepath.Join(dir, "case.json"))
	if err != nil {
		return LoadedCase{}, fmt.Errorf("read case metadata: %w", err)
	}
	var meta Case
	if err := json.Unmarshal(caseData, &meta); err != nil {
		return LoadedCase{}, fmt.Errorf("parse case metadata: %w", err)
	}
	if meta.ID == "" || meta.Description == "" {
		return LoadedCase{}, fmt.Errorf("case id and description are required")
	}
	if meta.Tier != "conformance" && meta.Tier != "capability" {
		return LoadedCase{}, fmt.Errorf("unknown tier %q", meta.Tier)
	}
	if meta.Comparison != "ordered" && meta.Comparison != "positional" {
		return LoadedCase{}, fmt.Errorf("unknown comparison %q", meta.Comparison)
	}

	input, err := readInput(dir)
	if err != nil {
		return LoadedCase{}, err
	}
	schema, err := os.ReadFile(filepath.Join(dir, "schema.json"))
	if err != nil {
		return LoadedCase{}, fmt.Errorf("read schema: %w", err)
	}
	expected, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	if err != nil {
		return LoadedCase{}, fmt.Errorf("read expected output: %w", err)
	}
	if err := validateExpected(schema, expected); err != nil {
		return LoadedCase{}, fmt.Errorf("validate expected output: %w", err)
	}
	return LoadedCase{Case: meta, Input: input, Schema: schema, Expected: expected}, nil
}

func readInput(dir string) ([]byte, error) {
	for _, name := range []string{"input.txt", "input.bin"} {
		input, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			return input, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
	}
	return nil, fmt.Errorf("case has no input.txt or input.bin")
}

func validateExpected(schemaData, expectedData []byte) error {
	var schemaDoc any
	if err := json.Unmarshal(schemaData, &schemaDoc); err != nil {
		return fmt.Errorf("invalid schema JSON: %w", err)
	}
	var expected any
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		return fmt.Errorf("invalid expected JSON: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schemaDoc); err != nil {
		return err
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}
	return schema.Validate(expected)
}

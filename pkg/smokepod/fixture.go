package smokepod

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FixtureFile struct {
	Source           string                      `json:"source"`
	RecordedWith     string                      `json:"recorded_with"`
	RecordedWithArgs []string                    `json:"recorded_with_args,omitempty"`
	Platform         PlatformInfo                `json:"platform"`
	Sections         map[string][]FixtureCommand `json:"sections"`
}

type FixtureCommand struct {
	Line     int    `json:"line"`
	Command  string `json:"command"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

type PlatformInfo struct {
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	ShellVersion string `json:"shell_version"`
}

func WriteFixture(path string, fixture *FixtureFile, indent string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, fmt.Errorf("creating fixture directory: %w", err)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)

	if err := enc.Encode(fixture); err != nil {
		return false, fmt.Errorf("marshaling fixture: %w", err)
	}

	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, buf.Bytes()) {
		return false, nil
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return false, fmt.Errorf("writing fixture: %w", err)
	}

	return true, nil
}

func ReadFixture(path string) (*FixtureFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading fixture: %w", err)
	}

	var fixture FixtureFile
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("parsing fixture: %w", err)
	}

	return &fixture, nil
}

func FixturePathFromTest(testPath, testsDir, fixturesDir string) string {
	relPath := testPath
	if testsDir != "" {
		info, err := os.Stat(testsDir)
		if err == nil && !info.IsDir() {
			testsDir = filepath.Dir(testsDir)
		}
		rel, err := filepath.Rel(testsDir, testPath)
		if err == nil {
			relPath = rel
		}
	}

	ext := filepath.Ext(relPath)
	base := relPath[:len(relPath)-len(ext)]

	return filepath.Join(fixturesDir, base+".fixture.json")
}

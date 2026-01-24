package smokepod

import (
	"fmt"
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

// Config represents the top-level configuration.
type Config struct {
	Name     string           `yaml:"name"`
	Version  string           `yaml:"version"`
	Settings Settings         `yaml:"settings"`
	Tests    []TestDefinition `yaml:"tests"`
}

// Settings holds global test settings.
type Settings struct {
	Timeout  time.Duration `yaml:"timeout"`
	Parallel *bool         `yaml:"parallel"`
	FailFast bool          `yaml:"fail_fast"`
}

// IsParallel returns whether tests should run in parallel (defaults to true).
func (s Settings) IsParallel() bool {
	if s.Parallel == nil {
		return true
	}
	return *s.Parallel
}

// TestDefinition represents a single test configuration.
type TestDefinition struct {
	Name  string   `yaml:"name"`
	Type  string   `yaml:"type"`  // "cli" or "playwright"
	Image string   `yaml:"image"`
	File  string   `yaml:"file"` // for cli tests
	Run   []string `yaml:"run"`  // specific sections to run
	Path  string   `yaml:"path"` // for playwright tests
	Args  []string `yaml:"args"` // for playwright tests
}

// ParseConfig loads and validates a configuration file.
func ParseConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyDefaults(&cfg)

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DefaultPlaywrightImage is the default Docker image for Playwright tests.
const DefaultPlaywrightImage = "mcr.microsoft.com/playwright:latest"

func applyDefaults(cfg *Config) {
	if cfg.Settings.Timeout == 0 {
		cfg.Settings.Timeout = 5 * time.Minute
	}
	// Parallel defaults to true via IsParallel() method.

	// Apply default images for test types
	for i := range cfg.Tests {
		if cfg.Tests[i].Image == "" && cfg.Tests[i].Type == "playwright" {
			cfg.Tests[i].Image = DefaultPlaywrightImage
		}
	}
}

func validateConfig(cfg *Config) error {
	if cfg.Name == "" {
		return fmt.Errorf("config: name is required")
	}

	if cfg.Version != "1" {
		return fmt.Errorf("config: version must be \"1\", got %q", cfg.Version)
	}

	for i, test := range cfg.Tests {
		if err := validateTest(test, i); err != nil {
			return err
		}
	}

	return nil
}

func validateTest(test TestDefinition, index int) error {
	prefix := fmt.Sprintf("tests[%d]", index)
	if test.Name != "" {
		prefix = fmt.Sprintf("tests[%d] (%s)", index, test.Name)
	}

	if test.Name == "" {
		return fmt.Errorf("%s: name is required", prefix)
	}

	if test.Type == "" {
		return fmt.Errorf("%s: type is required", prefix)
	}

	if test.Type != "cli" && test.Type != "playwright" {
		return fmt.Errorf("%s: type must be \"cli\" or \"playwright\", got %q", prefix, test.Type)
	}

	if test.Image == "" && test.Type == "cli" {
		return fmt.Errorf("%s: image is required for cli tests", prefix)
	}

	if test.Type == "cli" && test.File == "" {
		return fmt.Errorf("%s: file is required for cli tests", prefix)
	}

	if test.Type == "playwright" && test.Path == "" {
		return fmt.Errorf("%s: path is required for playwright tests", prefix)
	}

	return nil
}

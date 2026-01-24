package smokepod

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testdataPath(name string) string {
	return filepath.Join("..", "..", "testdata", "fixtures", name)
}

func TestParseConfig_Valid(t *testing.T) {
	cfg, err := ParseConfig(testdataPath("valid.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name != "myapp-smoke" {
		t.Errorf("name = %q, want %q", cfg.Name, "myapp-smoke")
	}

	if cfg.Version != "1" {
		t.Errorf("version = %q, want %q", cfg.Version, "1")
	}

	if cfg.Settings.Timeout != 10*time.Minute {
		t.Errorf("timeout = %v, want %v", cfg.Settings.Timeout, 10*time.Minute)
	}

	if !cfg.Settings.IsParallel() {
		t.Error("parallel should be true")
	}

	if cfg.Settings.FailFast {
		t.Error("fail_fast should be false")
	}

	if len(cfg.Tests) != 2 {
		t.Fatalf("len(tests) = %d, want 2", len(cfg.Tests))
	}

	// CLI test
	cli := cfg.Tests[0]
	if cli.Name != "api-health" {
		t.Errorf("tests[0].name = %q, want %q", cli.Name, "api-health")
	}
	if cli.Type != "cli" {
		t.Errorf("tests[0].type = %q, want %q", cli.Type, "cli")
	}
	if cli.File != "tests/api.test" {
		t.Errorf("tests[0].file = %q, want %q", cli.File, "tests/api.test")
	}
	if len(cli.Run) != 2 || cli.Run[0] != "health" || cli.Run[1] != "version" {
		t.Errorf("tests[0].run = %v, want [health version]", cli.Run)
	}

	// Playwright test
	pw := cfg.Tests[1]
	if pw.Name != "ui-smoke" {
		t.Errorf("tests[1].name = %q, want %q", pw.Name, "ui-smoke")
	}
	if pw.Type != "playwright" {
		t.Errorf("tests[1].type = %q, want %q", pw.Type, "playwright")
	}
	if pw.Path != "tests/e2e" {
		t.Errorf("tests[1].path = %q, want %q", pw.Path, "tests/e2e")
	}
}

func TestParseConfig_MissingName(t *testing.T) {
	_, err := ParseConfig(testdataPath("missing-name.yaml"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "name is required")
	}
}

func TestParseConfig_InvalidType(t *testing.T) {
	_, err := ParseConfig(testdataPath("invalid-type.yaml"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "type must be") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "type must be")
	}
}

func TestParseConfig_DefaultSettings(t *testing.T) {
	cfg, err := ParseConfig(testdataPath("minimal.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Settings.Timeout != 5*time.Minute {
		t.Errorf("default timeout = %v, want %v", cfg.Settings.Timeout, 5*time.Minute)
	}

	if !cfg.Settings.IsParallel() {
		t.Error("default parallel should be true")
	}

	if cfg.Settings.FailFast {
		t.Error("default fail_fast should be false")
	}
}

func TestParseConfig_CLIMissingFile(t *testing.T) {
	_, err := ParseConfig(testdataPath("cli-missing-file.yaml"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "file is required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "file is required")
	}
}

func TestParseConfig_PlaywrightMissingPath(t *testing.T) {
	_, err := ParseConfig(testdataPath("playwright-missing-path.yaml"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "path is required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "path is required")
	}
}

func TestParseConfig_FileNotFound(t *testing.T) {
	_, err := ParseConfig("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "reading config") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "reading config")
	}
}

func TestParseConfig_PlaywrightDefaultImage(t *testing.T) {
	cfg, err := ParseConfig(testdataPath("playwright-default-image.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Tests) != 1 {
		t.Fatalf("len(tests) = %d, want 1", len(cfg.Tests))
	}

	pw := cfg.Tests[0]
	if pw.Image != DefaultPlaywrightImage {
		t.Errorf("image = %q, want %q", pw.Image, DefaultPlaywrightImage)
	}
}

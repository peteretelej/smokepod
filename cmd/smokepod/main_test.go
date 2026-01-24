package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peteretelej/smokepod/pkg/smokepod"
	"github.com/urfave/cli/v2"
)

func testdataPath(name string) string {
	return filepath.Join("..", "..", "testdata", "fixtures", name)
}

func newTestApp() *cli.App {
	return &cli.App{
		Name:    "smokepod",
		Version: smokepod.VersionString(),
		Commands: []*cli.Command{
			runCommand(),
			validateCommand(),
		},
	}
}

func init() {
	// Override global cli error writer for tests
	cli.ErrWriter = io.Discard
	// Prevent os.Exit from being called in tests
	cli.OsExiter = func(code int) {}
}

// quietTestApp returns an app with suppressed output for error tests.
func quietTestApp() *cli.App {
	app := newTestApp()
	app.Writer = io.Discard
	app.ErrWriter = io.Discard
	return app
}

func TestCLI_Validate_Valid(t *testing.T) {
	app := newTestApp()

	// Capture stderr for validation output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := app.Run([]string{"smokepod", "validate", testdataPath("valid.yaml")})

	_ = w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "Config is valid") {
		t.Errorf("output = %q, want to contain %q", output, "Config is valid")
	}

	if !strings.Contains(output, "myapp-smoke") {
		t.Errorf("output = %q, want to contain config name", output)
	}
}

func TestCLI_Validate_Invalid(t *testing.T) {
	app := quietTestApp()

	err := app.Run([]string{"smokepod", "validate", testdataPath("missing-name.yaml")})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Check exit code
	exitErr, ok := err.(cli.ExitCoder)
	if !ok {
		t.Fatalf("expected cli.ExitCoder, got %T", err)
	}
	if exitErr.ExitCode() != exitConfigError {
		t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), exitConfigError)
	}
}

func TestCLI_Validate_MissingFile(t *testing.T) {
	app := quietTestApp()

	err := app.Run([]string{"smokepod", "validate", "nonexistent.yaml"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	exitErr, ok := err.(cli.ExitCoder)
	if !ok {
		t.Fatalf("expected cli.ExitCoder, got %T", err)
	}
	if exitErr.ExitCode() != exitConfigError {
		t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), exitConfigError)
	}

	if !strings.Contains(err.Error(), "Config file not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "Config file not found")
	}
}

func TestCLI_Validate_NoArgs(t *testing.T) {
	app := quietTestApp()

	err := app.Run([]string{"smokepod", "validate"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	exitErr, ok := err.(cli.ExitCoder)
	if !ok {
		t.Fatalf("expected cli.ExitCoder, got %T", err)
	}
	if exitErr.ExitCode() != exitConfigError {
		t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), exitConfigError)
	}
}

func TestCLI_Run_NoArgs(t *testing.T) {
	app := quietTestApp()

	err := app.Run([]string{"smokepod", "run"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	exitErr, ok := err.(cli.ExitCoder)
	if !ok {
		t.Fatalf("expected cli.ExitCoder, got %T", err)
	}
	if exitErr.ExitCode() != exitConfigError {
		t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), exitConfigError)
	}
}

func TestCLI_Run_MissingFile(t *testing.T) {
	app := quietTestApp()

	err := app.Run([]string{"smokepod", "run", "nonexistent.yaml"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	exitErr, ok := err.(cli.ExitCoder)
	if !ok {
		t.Fatalf("expected cli.ExitCoder, got %T", err)
	}
	if exitErr.ExitCode() != exitConfigError {
		t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), exitConfigError)
	}
}

func TestCLI_Run_InvalidConfig(t *testing.T) {
	app := quietTestApp()

	err := app.Run([]string{"smokepod", "run", testdataPath("missing-name.yaml")})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	exitErr, ok := err.(cli.ExitCoder)
	if !ok {
		t.Fatalf("expected cli.ExitCoder, got %T", err)
	}
	if exitErr.ExitCode() != exitConfigError {
		t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), exitConfigError)
	}
}

func TestCLI_Version(t *testing.T) {
	app := newTestApp()

	// Version flag causes os.Exit, so we just verify the app has version set
	if app.Version == "" {
		t.Error("app version should not be empty")
	}

	if !strings.Contains(app.Version, "dev") && !strings.Contains(app.Version, "commit") {
		t.Errorf("version = %q, want to contain version info", app.Version)
	}
}

func TestConfigDir(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"config.yaml", "."},
		{"./config.yaml", "."},
		{"path/to/config.yaml", "path/to"},
		{"/absolute/path/config.yaml", "/absolute/path"},
	}

	for _, tt := range tests {
		got := configDir(tt.path)
		if got != tt.want {
			t.Errorf("configDir(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

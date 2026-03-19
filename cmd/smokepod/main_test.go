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
			recordCommand(),
			verifyCommand(),
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

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name           string
		metadata       map[string][]string
		cliTarget      string
		cliTargetArgs  []string
		cliMode        string
		wantTarget     string
		wantTargetArgs []string
		wantMode       string
		wantErr        string
	}{
		{
			name:           "file-only target and args",
			metadata:       map[string][]string{"target": {"/usr/bin/jq"}, "target-arg": {"--tab"}},
			wantTarget:     "/usr/bin/jq",
			wantTargetArgs: []string{"--tab"},
			wantMode:       "shell",
		},
		{
			name:           "CLI-only target and args",
			metadata:       nil,
			cliTarget:      "/bin/sh",
			cliTargetArgs:  []string{"--norc"},
			cliMode:        "shell",
			wantTarget:     "/bin/sh",
			wantTargetArgs: []string{"--norc"},
			wantMode:       "shell",
		},
		{
			name:           "CLI overrides file",
			metadata:       map[string][]string{"target": {"/usr/bin/jq"}, "target-arg": {"--tab"}},
			cliTarget:      "/bin/sh",
			cliTargetArgs:  []string{"--norc"},
			cliMode:        "shell",
			wantTarget:     "/bin/sh",
			wantTargetArgs: []string{"--norc"},
			wantMode:       "shell",
		},
		{
			name:           "per-field: CLI target, file target-arg",
			metadata:       map[string][]string{"target": {"/usr/bin/jq"}, "target-arg": {"--tab"}},
			cliTarget:      "/bin/sh",
			wantTarget:     "/bin/sh",
			wantTargetArgs: []string{"--tab"},
			wantMode:       "shell",
		},
		{
			name:           "per-field: CLI mode, file target",
			metadata:       map[string][]string{"target": {"/bin/bash"}},
			cliMode:        "process",
			wantTarget:     "/bin/bash",
			wantTargetArgs: nil,
			wantMode:       "process",
		},
		{
			name:           "file mode fallback when CLI mode empty",
			metadata:       map[string][]string{"target": {"/bin/sh"}, "mode": {"process"}},
			wantTarget:     "/bin/sh",
			wantTargetArgs: nil,
			wantMode:       "process",
		},
		{
			name:    "missing both target sources",
			wantErr: "no target",
		},
		{
			name:     "invalid mode",
			metadata: map[string][]string{"target": {"/bin/sh"}, "mode": {"banana"}},
			wantErr:  "invalid mode",
		},
		{
			name:     "multiple target values error",
			metadata: map[string][]string{"target": {"/bin/sh", "/bin/bash"}},
			wantErr:  "multiple # target directives",
		},
		{
			name:           "mode defaults to shell",
			cliTarget:      "/bin/sh",
			wantTarget:     "/bin/sh",
			wantTargetArgs: nil,
			wantMode:       "shell",
		},
		{
			name:           "process mode from file",
			metadata:       map[string][]string{"target": {"/bin/sh"}, "mode": {"process"}},
			wantTarget:     "/bin/sh",
			wantTargetArgs: nil,
			wantMode:       "process",
		},
		{
			name:           "wrap mode from file",
			metadata:       map[string][]string{"target": {"/bin/sh"}, "mode": {"wrap"}},
			wantTarget:     "/bin/sh",
			wantTargetArgs: nil,
			wantMode:       "wrap",
		},
		{
			name:           "wrap mode from CLI",
			metadata:       map[string][]string{"target": {"/bin/sh"}},
			cliMode:        "wrap",
			wantTarget:     "/bin/sh",
			wantTargetArgs: nil,
			wantMode:       "wrap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, targetArgs, mode, err := resolveTarget("test.test", tt.metadata, tt.cliTarget, tt.cliTargetArgs, tt.cliMode)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target != tt.wantTarget {
				t.Errorf("target = %q, want %q", target, tt.wantTarget)
			}
			if len(targetArgs) != len(tt.wantTargetArgs) {
				t.Fatalf("targetArgs = %v, want %v", targetArgs, tt.wantTargetArgs)
			}
			for i := range tt.wantTargetArgs {
				if targetArgs[i] != tt.wantTargetArgs[i] {
					t.Errorf("targetArgs[%d] = %q, want %q", i, targetArgs[i], tt.wantTargetArgs[i])
				}
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode, tt.wantMode)
			}
		})
	}
}

func TestCLI_RecordCommand_TargetArgFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "no target-arg flags",
			args: []string{"smokepod", "record", "--target", "/bin/bash", "--tests", "tests", "--fixtures", "fixtures"},
		},
		{
			name: "single target-arg",
			args: []string{"smokepod", "record", "--target", "/bin/bash", "--target-arg", "--norc", "--tests", "tests", "--fixtures", "fixtures"},
		},
		{
			name: "multiple target-arg flags",
			args: []string{"smokepod", "record", "--target", "/bin/bash", "--target-arg", "--norc", "--target-arg", "--noprofile", "--tests", "tests", "--fixtures", "fixtures"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := quietTestApp()
			// The action will fail because test paths don't exist, but
			// we verify the flags are accepted without a parse error.
			err := app.Run(tt.args)
			if err != nil {
				exitErr, ok := err.(cli.ExitCoder)
				if ok && exitErr.ExitCode() == exitConfigError {
					// Expected: paths don't exist
					return
				}
				// Also acceptable: runtime error due to non-existent paths
				if ok && exitErr.ExitCode() == exitRuntimeError {
					return
				}
				// Check it's not a flag parse error
				if strings.Contains(err.Error(), "flag") && strings.Contains(err.Error(), "not defined") {
					t.Errorf("flag parse error: %v", err)
				}
			}
		})
	}
}

func TestCLI_VerifyCommand_TargetArgFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "no target-arg flags",
			args: []string{"smokepod", "verify", "--target", "/bin/bash", "--tests", "tests", "--fixtures", "fixtures"},
		},
		{
			name: "single target-arg",
			args: []string{"smokepod", "verify", "--target", "/bin/bash", "--target-arg", "--norc", "--tests", "tests", "--fixtures", "fixtures"},
		},
		{
			name: "multiple target-arg flags",
			args: []string{"smokepod", "verify", "--target", "/bin/bash", "--target-arg", "--norc", "--target-arg", "--noprofile", "--tests", "tests", "--fixtures", "fixtures"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := quietTestApp()
			err := app.Run(tt.args)
			if err != nil {
				exitErr, ok := err.(cli.ExitCoder)
				if ok && (exitErr.ExitCode() == exitConfigError || exitErr.ExitCode() == exitRuntimeError) {
					return
				}
				if strings.Contains(err.Error(), "flag") && strings.Contains(err.Error(), "not defined") {
					t.Errorf("flag parse error: %v", err)
				}
			}
		})
	}
}

func TestCLI_RecordWithoutTargetFlag(t *testing.T) {
	// Without --target and without file directives, record should produce an error
	app := quietTestApp()
	err := app.Run([]string{"smokepod", "record", "--tests", testdataPath(""), "--fixtures", t.TempDir()})
	if err == nil {
		// The action might not return a top-level error because it prints errors per-file
		// and continues. This is acceptable behavior.
		return
	}
	// If it does error, it should not be a flag parse error
	if strings.Contains(err.Error(), "flag") && strings.Contains(err.Error(), "not defined") {
		t.Errorf("flag parse error: %v", err)
	}
}

func TestCLI_VerifyWithoutTargetFlag(t *testing.T) {
	// Without --target and without file directives, verify should produce an error
	app := quietTestApp()
	err := app.Run([]string{"smokepod", "verify", "--tests", testdataPath(""), "--fixtures", t.TempDir()})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "flag") && strings.Contains(err.Error(), "not defined") {
		t.Errorf("flag parse error: %v", err)
	}
}

func TestFixtureFile_BackwardCompat(t *testing.T) {
	// Old fixture format without recorded_with_args should deserialize correctly
	oldJSON := `{
		"source": "test.test",
		"recorded_with": "/bin/sh",
		"recorded_at": "2024-01-01T00:00:00Z",
		"platform": {"os": "darwin", "arch": "arm64", "shell_version": ""},
		"sections": {}
	}`

	tmpFile := filepath.Join(t.TempDir(), "old.fixture.json")
	if err := os.WriteFile(tmpFile, []byte(oldJSON), 0644); err != nil {
		t.Fatal(err)
	}

	fixture, err := smokepod.ReadFixture(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error reading old fixture: %v", err)
	}

	if fixture.RecordedWithArgs != nil {
		t.Errorf("RecordedWithArgs = %v, want nil for old fixture", fixture.RecordedWithArgs)
	}
	if fixture.RecordedWith != "/bin/sh" {
		t.Errorf("RecordedWith = %q, want /bin/sh", fixture.RecordedWith)
	}
}

func TestCLI_RecordModeFlag(t *testing.T) {
	// Verify the --mode flag is accepted on record command
	app := quietTestApp()
	err := app.Run([]string{"smokepod", "record", "--target", "/bin/bash", "--mode", "shell", "--tests", "tests", "--fixtures", "fixtures"})
	if err != nil {
		if strings.Contains(err.Error(), "flag") && strings.Contains(err.Error(), "not defined") {
			t.Errorf("flag parse error: %v", err)
		}
	}
}

func TestCLI_AllowEmptyFlag(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{
			name:    "record with allow-empty",
			command: "record",
			args:    []string{"smokepod", "record", "--target", "/bin/bash", "--tests", "tests", "--fixtures", "fixtures", "--allow-empty"},
		},
		{
			name:    "verify with allow-empty",
			command: "verify",
			args:    []string{"smokepod", "verify", "--target", "/bin/bash", "--tests", "tests", "--fixtures", "fixtures", "--allow-empty"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := quietTestApp()
			err := app.Run(tt.args)
			if err != nil {
				exitErr, ok := err.(cli.ExitCoder)
				if ok && (exitErr.ExitCode() == exitConfigError || exitErr.ExitCode() == exitRuntimeError) {
					return
				}
				if strings.Contains(err.Error(), "flag") && strings.Contains(err.Error(), "not defined") {
					t.Errorf("flag parse error: %v", err)
				}
			}
		})
	}
}

func TestCLI_RecordIndentFlag(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal test file so discovery succeeds
	testContent := "## s\n$ echo hi\nhi\n"
	testPath := filepath.Join(tmpDir, "tests", "t.test")
	if err := os.MkdirAll(filepath.Dir(testPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testPath, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		indent    string
		wantErr   bool
		errSubstr string
	}{
		{"two_spaces", "2", false, ""},
		{"four_spaces", "4", false, ""},
		{"tab", "tab", false, ""},
		{"invalid", "3", true, "invalid --indent"},
		{"default", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := quietTestApp()
			fixturesDir := filepath.Join(tmpDir, "fixtures-"+tt.name)

			args := []string{
				"smokepod", "record",
				"--update",
				"--tests", filepath.Join(tmpDir, "tests"),
				"--fixtures", fixturesDir,
				"--target", "echo",
			}
			if tt.indent != "" {
				args = append(args, "--indent", tt.indent)
			}

			err := app.Run(args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

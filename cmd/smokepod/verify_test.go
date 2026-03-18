package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peteretelej/smokepod/pkg/smokepod"
	"github.com/urfave/cli/v2"
)

// writeTestFile creates a .test file with the given content.
func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeFixture creates a fixture JSON file with the given sections.
func writeFixture(t *testing.T, dir, name string, sections map[string][]smokepod.FixtureCommand) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	fixture := &smokepod.FixtureFile{
		Source:       "test",
		RecordedWith: "echo",
		RecordedAt:   time.Now(),
		Sections:     sections,
	}
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runApp(args ...string) error {
	app := quietTestApp()
	return app.Run(args)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(cli.ExitCoder); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func TestVerify_MissingFixtureSection(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test file has section "foo"
	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo hello\nhello\n")

	// fixture has NO section "foo"
	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if exitCode(err) != exitTestFailure {
		t.Errorf("expected exit code %d, got %d (err: %v)", exitTestFailure, exitCode(err), err)
	}
}

func TestVerify_StaleExtraFixtureSection(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test file has only section "foo"
	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo hello\nhello\n")

	// fixture has "foo" AND "bar" (bar is stale)
	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"foo": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
		"bar": {{Line: 5, Command: "echo stale", Stdout: "stale\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if exitCode(err) != exitTestFailure {
		t.Errorf("expected exit code %d, got %d (err: %v)", exitTestFailure, exitCode(err), err)
	}
}

func TestVerify_StaleExtraFixtureCommand(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test has 2 commands in section "foo"
	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo a\na\n\n$ echo b\nb\n")

	// fixture has 3 commands for section "foo"
	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"foo": {
			{Line: 2, Command: "echo a", Stdout: "a\n", ExitCode: 0},
			{Line: 4, Command: "echo b", Stdout: "b\n", ExitCode: 0},
			{Line: 6, Command: "echo c", Stdout: "c\n", ExitCode: 0},
		},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if exitCode(err) != exitTestFailure {
		t.Errorf("expected exit code %d, got %d (err: %v)", exitTestFailure, exitCode(err), err)
	}
}

func TestVerify_FewerFixtureCommands(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test has 3 commands in section "foo"
	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo a\na\n\n$ echo b\nb\n\n$ echo c\nc\n")

	// fixture has 2 commands for section "foo"
	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"foo": {
			{Line: 2, Command: "echo a", Stdout: "a\n", ExitCode: 0},
			{Line: 4, Command: "echo b", Stdout: "b\n", ExitCode: 0},
		},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if exitCode(err) != exitTestFailure {
		t.Errorf("expected exit code %d, got %d (err: %v)", exitTestFailure, exitCode(err), err)
	}
}

func TestVerify_EmptyDiscovery_NoFlag(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// No .test files at all
	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if exitCode(err) != exitConfigError {
		t.Errorf("expected exit code %d, got %d (err: %v)", exitConfigError, exitCode(err), err)
	}
	if err != nil && !strings.Contains(err.Error(), "no .test files found") {
		t.Errorf("expected error to mention 'no .test files found', got: %v", err)
	}
}

func TestVerify_EmptyDiscovery_AllowEmpty(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// No .test files, but --allow-empty is set
	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
		"--allow-empty",
	)
	if err != nil {
		t.Errorf("expected nil error with --allow-empty, got: %v", err)
	}
}

func TestVerify_PartialRun_ExtraUnselected(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test has section "foo"
	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo hello\nhello\n")

	// fixture has "foo" and "bar"
	// With --run=foo, "bar" is outside selected scope and should be ignored
	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"foo": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
		"bar": {{Line: 5, Command: "echo extra", Stdout: "extra\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
		"--run", "foo",
	)
	if err != nil {
		t.Errorf("expected success with --run=foo (extra unselected 'bar' ignored), got: %v", err)
	}
}

func TestVerify_PartialRun_StaleSelected(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test has section "foo" only
	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo hello\nhello\n")

	// fixture has "foo" and "bar"
	// With --run=foo,bar — "bar" is in selected scope but not in .test → stale → fail
	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"foo": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
		"bar": {{Line: 5, Command: "echo stale", Stdout: "stale\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
		"--run", "foo,bar",
	)
	if exitCode(err) != exitTestFailure {
		t.Errorf("expected exit code %d for stale selected section, got %d (err: %v)", exitTestFailure, exitCode(err), err)
	}
}

func TestVerify_XFail_ExpectedFailure(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// Section marked (xfail), command output won't match fixture
	writeTestFile(t, testsDir, "example.test", "## broken (xfail)\n$ echo actual\nactual\n")

	// Fixture records different output, so verify will see a mismatch
	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"broken": {{Line: 2, Command: "echo actual", Stdout: "wrong\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if err != nil {
		t.Errorf("expected success (xfail section failure is expected), got: %v", err)
	}
}

func TestVerify_XPass_UnexpectedPass(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// Section marked (xfail), but output actually matches fixture
	writeTestFile(t, testsDir, "example.test", "## broken (xfail)\n$ echo hello\nhello\n")

	// Fixture matches actual output, triggering xpass
	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"broken": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
		"--json",
	)
	if exitCode(err) != exitTestFailure {
		t.Errorf("expected exit code %d for xpass, got %d (err: %v)", exitTestFailure, exitCode(err), err)
	}
}

func TestVerify_XFail_WithReason(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// Section with xfail reason, output won't match
	writeTestFile(t, testsDir, "example.test", "## broken (xfail: known bug #99)\n$ echo actual\nactual\n")

	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"broken": {{Line: 2, Command: "echo actual", Stdout: "wrong\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if err != nil {
		t.Errorf("expected success (xfail with reason), got: %v", err)
	}
}

func TestVerify_XFail_PartialPass(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// xfail section with 2 commands: one matches, one doesn't
	writeTestFile(t, testsDir, "example.test", "## partial (xfail)\n$ echo match\nmatch\n\n$ echo actual\nactual\n")

	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"partial": {
			{Line: 2, Command: "echo match", Stdout: "match\n", ExitCode: 0},
			{Line: 5, Command: "echo actual", Stdout: "wrong\n", ExitCode: 0},
		},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if err != nil {
		t.Errorf("expected success (partial xfail counts as xfail), got: %v", err)
	}
}

func TestVerify_XFail_MixedWithNormal(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// One normal passing section + one xfail section
	writeTestFile(t, testsDir, "example.test",
		"## normal\n$ echo hello\nhello\n\n## broken (xfail)\n$ echo actual\nactual\n")

	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"normal": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
		"broken": {{Line: 5, Command: "echo actual", Stdout: "wrong\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if err != nil {
		t.Errorf("expected success (normal pass + xfail = passing suite), got: %v", err)
	}
}

func TestVerify_XFail_MixedWithFailure(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// One normal section that fails + one xfail section
	writeTestFile(t, testsDir, "example.test",
		"## normal\n$ echo actual\nactual\n\n## broken (xfail)\n$ echo actual\nactual\n")

	writeFixture(t, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"normal": {{Line: 2, Command: "echo actual", Stdout: "wrong\n", ExitCode: 0}},
		"broken": {{Line: 5, Command: "echo actual", Stdout: "wrong\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if exitCode(err) != exitTestFailure {
		t.Errorf("expected exit code %d (normal failure present), got %d (err: %v)", exitTestFailure, exitCode(err), err)
	}
}

func TestRecord_EmptyDiscovery_NoFlag(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	err := runApp("smokepod", "record",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
		"--update",
	)
	if exitCode(err) != exitConfigError {
		t.Errorf("expected exit code %d, got %d (err: %v)", exitConfigError, exitCode(err), err)
	}
	if err != nil && !strings.Contains(err.Error(), "no .test files found") {
		t.Errorf("expected error to mention 'no .test files found', got: %v", err)
	}
}

func TestRecord_EmptyDiscovery_AllowEmpty(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	err := runApp("smokepod", "record",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
		"--update",
		"--allow-empty",
	)
	if err != nil {
		t.Errorf("expected nil error with --allow-empty, got: %v", err)
	}
}

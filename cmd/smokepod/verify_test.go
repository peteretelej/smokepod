package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peteretelej/smokepod/pkg/smokepod"
	"github.com/urfave/cli/v3"
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

// writeFixture creates a fixture JSON file with the given sections and
// records it (plus the tests tree) in the fixtures manifest, matching what
// `record` produces. Verify requires a manifest since baseline protection.
func writeFixture(t *testing.T, testsDir, fixturesDir, name string, sections map[string][]smokepod.FixtureCommand) string {
	t.Helper()
	path := filepath.Join(fixturesDir, name)
	fixture := &smokepod.FixtureFile{
		Source:       "test",
		RecordedWith: "echo",
		Sections:     sections,
	}
	if _, err := smokepod.WriteFixture(path, fixture, "  "); err != nil {
		t.Fatal(err)
	}
	manifest, err := smokepod.ScanManifest(testsDir, fixturesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := smokepod.WriteManifest(fixturesDir, manifest); err != nil {
		t.Fatal(err)
	}
	return path
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns the
// captured output.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	return buf.String()
}

func runApp(args ...string) error {
	app := quietTestApp()
	return app.Run(context.Background(), args)
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
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{})

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
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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

	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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

	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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

	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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

	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
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

func TestVerify_RegexMatch(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test uses (re) pattern for date
	writeTestFile(t, testsDir, "example.test",
		"## dates\n$ date +%Y-%m-%d\n\\d{4}-\\d{2}-\\d{2} (re)\n")

	// Fixture has a specific recorded date
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
		"dates": {{Line: 2, Command: "date +%Y-%m-%d", Stdout: "2026-03-18\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	// Actual output will be today's date, regex should match any date
	if err != nil {
		t.Errorf("expected success (regex should match date), got: %v", err)
	}
}

func TestVerify_RegexMismatch(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test uses (re) that won't match "not-a-number"
	writeTestFile(t, testsDir, "example.test",
		"## nums\n$ echo not-a-number\n^\\d+$ (re)\n")

	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
		"nums": {{Line: 2, Command: "echo not-a-number", Stdout: "42\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if exitCode(err) != exitTestFailure {
		t.Errorf("expected exit code %d (regex mismatch), got %d (err: %v)", exitTestFailure, exitCode(err), err)
	}
}

func TestVerify_NoExpectations_BackwardCompat(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test has no expected output lines
	writeTestFile(t, testsDir, "example.test", "## basic\n$ echo hello\nhello\n")

	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
		"basic": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if err != nil {
		t.Errorf("expected success (literal match, backward compat), got: %v", err)
	}
}

func TestVerify_MixedRegexAndLiteral(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// 3 lines: line 1 literal, line 2 regex, line 3 literal
	writeTestFile(t, testsDir, "example.test",
		"## mixed\n$ printf 'header\\n42\\nfooter\\n'\nheader\n\\d+ (re)\nfooter\n")

	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
		"mixed": {{Line: 2, Command: "printf 'header\\n42\\nfooter\\n'", Stdout: "header\n999\nfooter\n", ExitCode: 0}},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if err != nil {
		t.Errorf("expected success (line 2 regex matches, others literal), got: %v", err)
	}
}

func TestVerify_PartialMatchWithMismatch(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	// .test has 3 commands in section "foo"
	writeTestFile(t, testsDir, "example.test",
		"## foo\n$ echo a\na\n\n$ echo b\nb\n\n$ echo c\nc\n")

	// fixture has only 2 commands - the first two match
	writeFixture(t, testsDir, fixturesDir,"example.fixture.json", map[string][]smokepod.FixtureCommand{
		"foo": {
			{Line: 2, Command: "echo a", Stdout: "a\n", ExitCode: 0},
			{Line: 5, Command: "echo b", Stdout: "b\n", ExitCode: 0},
		},
	})

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	// Still fails due to count mismatch
	if exitCode(err) != exitTestFailure {
		t.Errorf("expected exit code %d, got %d (err: %v)", exitTestFailure, exitCode(err), err)
	}
}

func TestVerify_ManifestDrift_NamesDriftedFile(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo hello\nhello\n")
	writeFixture(t, testsDir, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"foo": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
	})

	// Tamper: flip one recorded value without updating the manifest.
	fixturePath := filepath.Join(fixturesDir, "example.fixture.json")
	fixture, err := smokepod.ReadFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Sections["foo"][0].Stdout = "tampered\n"
	if _, err := smokepod.WriteFixture(fixturePath, fixture, "  "); err != nil {
		t.Fatal(err)
	}

	var output string
	var verifyErr error
	output = captureStderr(t, func() {
		verifyErr = runApp("smokepod", "verify",
			"--target", "/bin/sh",
			"--tests", testsDir,
			"--fixtures", fixturesDir,
		)
	})

	if exitCode(verifyErr) != exitTestFailure {
		t.Fatalf("expected exit code %d, got %d (err: %v)", exitTestFailure, exitCode(verifyErr), verifyErr)
	}
	if !strings.Contains(output, "example.fixture.json") {
		t.Errorf("output = %q, want it to name the drifted file", output)
	}
	if !strings.Contains(output, "MANIFEST DRIFT") {
		t.Errorf("output = %q, want manifest drift report", output)
	}
}

func TestVerify_MissingManifest(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo hello\nhello\n")

	// Fixture without a manifest: verify must refuse with an actionable
	// message instead of trusting unprotected fixtures.
	fixture := &smokepod.FixtureFile{
		Source:       "test",
		RecordedWith: "echo",
		Sections: map[string][]smokepod.FixtureCommand{
			"foo": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
		},
	}
	path := filepath.Join(fixturesDir, "example.fixture.json")
	if _, err := smokepod.WriteFixture(path, fixture, "  "); err != nil {
		t.Fatal(err)
	}

	err := runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if exitCode(err) != exitConfigError {
		t.Fatalf("expected exit code %d, got %d (err: %v)", exitConfigError, exitCode(err), err)
	}
	if err != nil && !strings.Contains(err.Error(), "manifest") {
		t.Errorf("error = %q, want it to mention the manifest", err.Error())
	}
}

func TestUpdate_ApprovalGate(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo hello\nhello\n")
	fixturePath := writeFixture(t, testsDir, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"foo": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
	})

	// Tamper after recording so the manifest reports drift.
	fixture, err := smokepod.ReadFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Sections["foo"][0].Stdout = "tampered\n"
	if _, err := smokepod.WriteFixture(fixturePath, fixture, "  "); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	// Non-interactive stdin (a pipe, never written to): update must refuse
	// and change nothing, even though drift exists.
	oldStdin := os.Stdin
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = pw.Close() // EOF on read; the prompt path must never block the test
	os.Stdin = pr
	refused := runApp("smokepod", "update",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	os.Stdin = oldStdin
	_ = pr.Close()

	if refused == nil {
		t.Fatal("expected non-zero exit without approval, got nil")
	}
	after, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("fixtures changed without approval")
	}

	// --yes: re-records from the real target and refreshes the manifest.
	err = runApp("smokepod", "update",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
		"--yes",
	)
	if err != nil {
		t.Fatalf("expected update --yes to succeed, got %v", err)
	}

	updated, err := smokepod.ReadFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Sections["foo"][0].Stdout != "hello\n" {
		t.Errorf("stdout = %q, want re-recorded \"hello\\n\"", updated.Sections["foo"][0].Stdout)
	}

	// Manifest refreshed: verify passes against the updated baseline again.
	err = runApp("smokepod", "verify",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if err != nil {
		t.Errorf("expected verify to pass after update, got %v", err)
	}
}

func TestProbe_DetectsTampering(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo hello\nhello\n")
	writeFixture(t, testsDir, fixturesDir, "example.fixture.json", map[string][]smokepod.FixtureCommand{
		"foo": {{Line: 2, Command: "echo hello", Stdout: "hello\n", ExitCode: 0}},
	})

	var output string
	var probeErr error
	output = captureStderr(t, func() {
		probeErr = runApp("smokepod", "probe",
			"--target", "/bin/sh",
			"--tests", testsDir,
			"--fixtures", fixturesDir,
		)
	})

	if probeErr != nil {
		t.Fatalf("expected probe success (tamper detected), got %v", probeErr)
	}
	if !strings.Contains(output, "protection verified") {
		t.Errorf("output = %q, want \"protection verified\"", output)
	}
}

func TestProbe_MissingManifest(t *testing.T) {
	testsDir := t.TempDir()
	fixturesDir := t.TempDir()

	writeTestFile(t, testsDir, "example.test", "## foo\n$ echo hello\nhello\n")

	err := runApp("smokepod", "probe",
		"--target", "/bin/sh",
		"--tests", testsDir,
		"--fixtures", fixturesDir,
	)
	if exitCode(err) != exitConfigError {
		t.Fatalf("expected exit code %d, got %d (err: %v)", exitConfigError, exitCode(err), err)
	}
}

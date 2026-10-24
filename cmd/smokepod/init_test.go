package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildStubTarget builds the deterministic stub binary from testdata/stub.
// The build runs from the repo root, so it works regardless of the test
// process's current directory.
func buildStubTarget(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repo root")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

	bin := filepath.Join(t.TempDir(), "stub")
	cmd := exec.Command("go", "build", "-o", bin, filepath.Join(root, "testdata", "stub", "main.go"))
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building stub target: %v: %s", err, out)
	}
	return bin
}

// chdirTemp moves the process into a fresh temp dir for the test duration.
// Init writes its conventional tests/ and fixtures/ dirs relative to the
// working directory, so tests must not run in the repo.
func chdirTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns the
// captured output.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	runErr := fn()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	return buf.String(), runErr
}

func TestCLI_Init_ScaffoldsRecordsAndPrintsNextCommands(t *testing.T) {
	chdirTemp(t)
	stub := buildStubTarget(t)

	out, err := captureStdout(t, func() error {
		return runApp("smokepod", "init", "--target", stub, "--dir", "tests", "--probe", "greet")
	})
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	for _, p := range []string{
		filepath.Join("tests", "smoke.test"),
		filepath.Join("fixtures", "smoke.fixture.json"),
		filepath.Join("fixtures", "manifest.json"),
	} {
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("init did not write %s: %v", p, statErr)
		}
	}

	for _, want := range []string{
		"smokepod verify --target " + stub,
		"smokepod update --target " + stub,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("next commands output = %q, want to contain %q", out, want)
		}
	}

	content, err := os.ReadFile(filepath.Join("tests", "smoke.test"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# normalize: timestamps,paths", "## version", "## help", "## greet"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("scaffold missing %q:\n%s", want, content)
		}
	}

	// Structural roundtrip: the printed verify command passes against the
	// freshly recorded baseline.
	var verifyErr error
	captureStderr(t, func() {
		verifyErr = runApp("smokepod", "verify", "--target", stub, "--tests", "tests", "--fixtures", "fixtures")
	})
	if verifyErr != nil {
		t.Fatalf("verify after init failed: %v", verifyErr)
	}
}

func TestCLI_Init_RefusesOverwriteWithoutForce(t *testing.T) {
	chdirTemp(t)
	stub := buildStubTarget(t)

	if err := runApp("smokepod", "init", "--target", stub, "--dir", "tests"); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	readFile := func(p string) string {
		t.Helper()
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	scaffold := readFile(filepath.Join("tests", "smoke.test"))
	manifest := readFile(filepath.Join("fixtures", "manifest.json"))
	fixture := readFile(filepath.Join("fixtures", "smoke.fixture.json"))

	err := runApp("smokepod", "init", "--target", stub, "--dir", "tests")
	if err == nil {
		t.Fatal("second init without --force should fail, got nil")
	}
	if code := exitCode(err); code != exitConfigError {
		t.Errorf("exit code = %d, want %d", code, exitConfigError)
	}
	if readFile(filepath.Join("tests", "smoke.test")) != scaffold {
		t.Error("refused init modified the .test file")
	}
	if readFile(filepath.Join("fixtures", "manifest.json")) != manifest {
		t.Error("refused init modified the manifest")
	}
	if readFile(filepath.Join("fixtures", "smoke.fixture.json")) != fixture {
		t.Error("refused init modified the fixture")
	}

	if err := runApp("smokepod", "init", "--target", stub, "--dir", "tests", "--force"); err != nil {
		t.Errorf("init --force failed: %v", err)
	}
}

func TestCLI_Init_NoTargetWritesCommentedTemplate(t *testing.T) {
	chdirTemp(t)

	if err := runApp("smokepod", "init", "--dir", "tests"); err != nil {
		t.Fatalf("bare init failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join("tests", "smoke.test"))
	if err != nil {
		t.Fatalf("scaffold not written: %v", err)
	}
	if !strings.Contains(string(content), "# normalize: timestamps,paths") {
		t.Errorf("template missing normalize directive:\n%s", content)
	}
	if !strings.Contains(string(content), "# ## version") {
		t.Errorf("template missing commented probe section:\n%s", content)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "## ") {
			t.Errorf("no-target template has active section %q, want all commented", line)
		}
	}

	if _, err := os.Stat("fixtures"); !os.IsNotExist(err) {
		t.Errorf("fixtures dir written for bare init, want none")
	}
}

func TestCLI_Init_JSONOutput(t *testing.T) {
	chdirTemp(t)
	stub := buildStubTarget(t)

	out, err := captureStdout(t, func() error {
		return runApp("smokepod", "init", "--target", stub, "--dir", "tests", "--json")
	})
	if err != nil {
		t.Fatalf("init --json failed: %v", err)
	}
	for _, want := range []string{`"files"`, `"next"`, "smoke.test", "smokepod verify --target"} {
		if !strings.Contains(out, want) {
			t.Errorf("json output = %q, want to contain %q", out, want)
		}
	}
}

package smokepod

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndReadFixture(t *testing.T) {
	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "test.fixture.json")

	fixture := &FixtureFile{
		Source:       "tests/test.test",
		RecordedWith: "/bin/bash",
		RecordedAt:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Platform: PlatformInfo{
			OS:           "darwin",
			Arch:         "arm64",
			ShellVersion: "5.2.21",
		},
		Sections: map[string][]FixtureCommand{
			"section1": {
				{
					Line:     5,
					Command:  "echo hello",
					Stdout:   "hello\n",
					Stderr:   "",
					ExitCode: 0,
				},
			},
		},
	}

	if err := WriteFixture(fixturePath, fixture); err != nil {
		t.Fatalf("WriteFixture failed: %v", err)
	}

	read, err := ReadFixture(fixturePath)
	if err != nil {
		t.Fatalf("ReadFixture failed: %v", err)
	}

	if read.Source != fixture.Source {
		t.Errorf("Source = %q, want %q", read.Source, fixture.Source)
	}

	if read.RecordedWith != fixture.RecordedWith {
		t.Errorf("RecordedWith = %q, want %q", read.RecordedWith, fixture.RecordedWith)
	}

	if len(read.Sections) != 1 {
		t.Fatalf("len(Sections) = %d, want 1", len(read.Sections))
	}

	commands := read.Sections["section1"]
	if len(commands) != 1 {
		t.Fatalf("len(commands) = %d, want 1", len(commands))
	}

	if commands[0].Command != "echo hello" {
		t.Errorf("Command = %q, want %q", commands[0].Command, "echo hello")
	}
}

func TestFixturePathFromTest(t *testing.T) {
	tests := []struct {
		testPath    string
		testsDir    string
		fixturesDir string
		expected    string
	}{
		{
			testPath:    "tests/comparison/pipes.test",
			testsDir:    "tests",
			fixturesDir: "fixtures",
			expected:    "fixtures/comparison/pipes.fixture.json",
		},
		{
			testPath:    "/abs/path/tests/test.test",
			testsDir:    "/abs/path/tests",
			fixturesDir: "fixtures",
			expected:    "fixtures/test.fixture.json",
		},
		{
			testPath:    "test.test",
			testsDir:    "",
			fixturesDir: "fixtures",
			expected:    "fixtures/test.fixture.json",
		},
	}

	for _, tc := range tests {
		result := FixturePathFromTest(tc.testPath, tc.testsDir, tc.fixturesDir)
		if result != tc.expected {
			t.Errorf("FixturePathFromTest(%q, %q, %q) = %q, want %q",
				tc.testPath, tc.testsDir, tc.fixturesDir, result, tc.expected)
		}
	}
}

func TestFixturePathFromTest_SingleFile(t *testing.T) {
	// When testsDir points to a single file, FixturePathFromTest should
	// use the file's directory as the base, not the file path itself.
	tmpDir := t.TempDir()

	// Create tests/foo.test
	testsDir := filepath.Join(tmpDir, "tests")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatal(err)
	}
	fooPath := filepath.Join(testsDir, "foo.test")
	if err := os.WriteFile(fooPath, []byte("## s\n$ echo hi\nhi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := FixturePathFromTest(fooPath, fooPath, "fixtures")
	want := filepath.Join("fixtures", "foo.fixture.json")
	if got != want {
		t.Errorf("FixturePathFromTest(single file) = %q, want %q", got, want)
	}

	// Create tests/sub/bar.test
	subDir := filepath.Join(testsDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	barPath := filepath.Join(subDir, "bar.test")
	if err := os.WriteFile(barPath, []byte("## s\n$ echo hi\nhi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got = FixturePathFromTest(barPath, barPath, "out")
	want = filepath.Join("out", "bar.fixture.json")
	if got != want {
		t.Errorf("FixturePathFromTest(single file nested) = %q, want %q", got, want)
	}
}

func TestWriteFixtureCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "subdir", "nested", "test.fixture.json")

	fixture := &FixtureFile{
		Source:       "test.test",
		RecordedWith: "/bin/bash",
		RecordedAt:   time.Now(),
		Platform:     PlatformInfo{},
		Sections:     map[string][]FixtureCommand{},
	}

	if err := WriteFixture(fixturePath, fixture); err != nil {
		t.Fatalf("WriteFixture failed: %v", err)
	}

	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Error("Fixture file was not created")
	}
}

package testfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testdataPath(name string) string {
	return filepath.Join("..", "..", "testdata", "fixtures", name)
}

func requireSection(t *testing.T, tf *TestFile, name string) *Section {
	t.Helper()
	section := tf.GetSection(name)
	if section == nil {
		t.Fatalf("section %q not found", name)
	}
	return section
}

func TestParse_SingleSection(t *testing.T) {
	tf, err := Parse(testdataPath("simple.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(tf.Sections))
	}

	section := requireSection(t, tf, "basic")

	if len(section.Commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(section.Commands))
	}

	cmd := section.Commands[0]
	if cmd.Cmd != "echo hello" {
		t.Errorf("cmd = %q, want %q", cmd.Cmd, "echo hello")
	}

	if len(cmd.Expected) != 1 {
		t.Fatalf("expected lines = %d, want 1", len(cmd.Expected))
	}

	if cmd.Expected[0].Text != "hello" {
		t.Errorf("expected[0] = %q, want %q", cmd.Expected[0].Text, "hello")
	}

	if cmd.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", cmd.ExitCode)
	}
}

func TestParse_MultipleSections(t *testing.T) {
	tf, err := Parse(testdataPath("multi-section.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Sections) != 3 {
		t.Fatalf("sections = %d, want 3", len(tf.Sections))
	}

	// Check order is preserved
	expected := []string{"first", "second", "third"}
	for i, name := range tf.Order {
		if name != expected[i] {
			t.Errorf("order[%d] = %q, want %q", i, name, expected[i])
		}
	}

	// Check each section exists
	for _, name := range expected {
		if tf.GetSection(name) == nil {
			t.Errorf("section %q not found", name)
		}
	}
}

func TestParse_RegexExpected(t *testing.T) {
	tf, err := Parse(testdataPath("regex.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	section := requireSection(t, tf, "version")
	cmd := section.Commands[0]
	if len(cmd.Expected) != 1 {
		t.Fatalf("expected lines = %d, want 1", len(cmd.Expected))
	}

	exp := cmd.Expected[0]
	if !exp.IsRegex {
		t.Error("expected[0] should be regex")
	}
	if exp.Text != "NAME=.*" {
		t.Errorf("expected[0].text = %q, want %q", exp.Text, "NAME=.*")
	}
}

func TestParse_ExitCode(t *testing.T) {
	tf, err := Parse(testdataPath("exit-codes.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		section  string
		exitCode int
	}{
		{"success", 0},
		{"failure", 1},
		{"custom", 42},
	}

	for _, tt := range tests {
		section := tf.GetSection(tt.section)
		if section == nil {
			t.Errorf("section %q not found", tt.section)
			continue
		}
		if len(section.Commands) != 1 {
			t.Errorf("section %q: commands = %d, want 1", tt.section, len(section.Commands))
			continue
		}
		if section.Commands[0].ExitCode != tt.exitCode {
			t.Errorf("section %q: exit code = %d, want %d", tt.section, section.Commands[0].ExitCode, tt.exitCode)
		}
	}
}

func TestParse_Comments(t *testing.T) {
	tf, err := Parse(testdataPath("comments.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	section := requireSection(t, tf, "main")

	if len(section.Commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(section.Commands))
	}

	cmd := section.Commands[0]
	if cmd.Cmd != "echo test" {
		t.Errorf("cmd = %q, want %q", cmd.Cmd, "echo test")
	}
}

func TestParse_MultilineOutput(t *testing.T) {
	tf, err := Parse(testdataPath("multiline.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	section := requireSection(t, tf, "lines")
	cmd := section.Commands[0]
	if len(cmd.Expected) != 3 {
		t.Fatalf("expected lines = %d, want 3", len(cmd.Expected))
	}

	expected := []string{"line1", "line2", "line3"}
	for i, exp := range cmd.Expected {
		if exp.Text != expected[i] {
			t.Errorf("expected[%d] = %q, want %q", i, exp.Text, expected[i])
		}
	}
}

func TestParse_EmptyFile(t *testing.T) {
	// Create a temp empty file
	f, err := os.CreateTemp("", "empty*.test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_ = f.Close()

	tf, err := Parse(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Sections) != 0 {
		t.Errorf("sections = %d, want 0", len(tf.Sections))
	}
}

func TestParse_NoSectionHeader(t *testing.T) {
	// Create a temp file with command but no section
	f, err := os.CreateTemp("", "nosection*.test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	if _, err := f.WriteString("$ echo hello\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	_, err = Parse(f.Name())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "command before section header") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "command before section header")
	}
}

func TestParse_PreservesOrder(t *testing.T) {
	tf, err := Parse(testdataPath("multi-section.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sections, err := tf.GetSections(nil)
	if err != nil {
		t.Fatalf("GetSections: %v", err)
	}

	if len(sections) != 3 {
		t.Fatalf("sections = %d, want 3", len(sections))
	}

	expected := []string{"first", "second", "third"}
	for i, s := range sections {
		if s.Name != expected[i] {
			t.Errorf("sections[%d].Name = %q, want %q", i, s.Name, expected[i])
		}
	}
}

func TestParse_DuplicateSection(t *testing.T) {
	// Create a temp file with duplicate sections
	f, err := os.CreateTemp("", "duplicate*.test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	if _, err := f.WriteString("## test\n$ echo one\n\n## test\n$ echo two\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	_, err = Parse(f.Name())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "duplicate section") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "duplicate section")
	}
}

func TestGetSections_Named(t *testing.T) {
	tf, err := Parse(testdataPath("multi-section.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sections, err := tf.GetSections([]string{"third", "first"})
	if err != nil {
		t.Fatalf("GetSections: %v", err)
	}

	if len(sections) != 2 {
		t.Fatalf("sections = %d, want 2", len(sections))
	}

	if sections[0].Name != "third" {
		t.Errorf("sections[0].Name = %q, want %q", sections[0].Name, "third")
	}
	if sections[1].Name != "first" {
		t.Errorf("sections[1].Name = %q, want %q", sections[1].Name, "first")
	}
}

func TestGetSections_NotFound(t *testing.T) {
	tf, err := Parse(testdataPath("simple.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = tf.GetSections([]string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "section not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "section not found")
	}
}

func TestParse_FileNotFound(t *testing.T) {
	_, err := Parse("nonexistent.test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "opening test file") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "opening test file")
	}
}

func TestParse_MultilineCommand(t *testing.T) {
	tf, err := Parse(testdataPath("multiline-cmd.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Multi-line for loop should be concatenated into one command
	section := requireSection(t, tf, "for-loop")
	if len(section.Commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(section.Commands))
	}

	cmd := section.Commands[0]
	wantCmd := "for i in 1 2 3; do\necho $i\ndone"
	if cmd.Cmd != wantCmd {
		t.Errorf("cmd = %q, want %q", cmd.Cmd, wantCmd)
	}

	if len(cmd.Expected) != 3 {
		t.Fatalf("expected lines = %d, want 3", len(cmd.Expected))
	}

	for i, want := range []string{"1", "2", "3"} {
		if cmd.Expected[i].Text != want {
			t.Errorf("expected[%d] = %q, want %q", i, cmd.Expected[i].Text, want)
		}
	}

	// Separate commands (with empty line between) should remain separate
	section = requireSection(t, tf, "separate-cmds")
	if len(section.Commands) != 2 {
		t.Fatalf("commands = %d, want 2", len(section.Commands))
	}

	if section.Commands[0].Cmd != "echo first" {
		t.Errorf("cmd[0] = %q, want %q", section.Commands[0].Cmd, "echo first")
	}
	if section.Commands[1].Cmd != "echo second" {
		t.Errorf("cmd[1] = %q, want %q", section.Commands[1].Cmd, "echo second")
	}
}

func TestParse_StderrSuffix(t *testing.T) {
	tf, err := Parse(testdataPath("stderr.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (stderr) suffix
	section := requireSection(t, tf, "stderr-only")
	cmd := section.Commands[0]
	if len(cmd.Expected) != 1 {
		t.Fatalf("expected lines = %d, want 1", len(cmd.Expected))
	}
	exp := cmd.Expected[0]
	if exp.Text != "error message" {
		t.Errorf("text = %q, want %q", exp.Text, "error message")
	}
	if !exp.IsStderr {
		t.Error("expected IsStderr=true")
	}
	if exp.IsRegex {
		t.Error("expected IsRegex=false")
	}

	// (stderr,re) combined suffix
	section = requireSection(t, tf, "stderr-regex")
	cmd = section.Commands[0]
	exp = cmd.Expected[0]
	if exp.Text != "warning: .*" {
		t.Errorf("text = %q, want %q", exp.Text, "warning: .*")
	}
	if !exp.IsStderr {
		t.Error("expected IsStderr=true")
	}
	if !exp.IsRegex {
		t.Error("expected IsRegex=true")
	}

	// Mixed stdout and stderr expectations
	section = requireSection(t, tf, "mixed")
	cmd = section.Commands[0]
	if len(cmd.Expected) != 2 {
		t.Fatalf("expected lines = %d, want 2", len(cmd.Expected))
	}
	if cmd.Expected[0].IsStderr {
		t.Error("expected[0] should be stdout")
	}
	if !cmd.Expected[1].IsStderr {
		t.Error("expected[1] should be stderr")
	}

	// (re,stderr) order should also work
	section = requireSection(t, tf, "re-stderr-order")
	cmd = section.Commands[0]
	exp = cmd.Expected[0]
	if !exp.IsRegex {
		t.Error("expected IsRegex=true for (re,stderr)")
	}
	if !exp.IsStderr {
		t.Error("expected IsStderr=true for (re,stderr)")
	}
}

func TestParse_LineNumbers(t *testing.T) {
	tf, err := Parse(testdataPath("simple.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	section := requireSection(t, tf, "basic")
	cmd := section.Commands[0]
	// Line 2 should be the command (line 1 is ## basic)
	if cmd.Line != 2 {
		t.Errorf("cmd.Line = %d, want 2", cmd.Line)
	}

	// Line 3 should be the expected output
	if cmd.Expected[0].Line != 3 {
		t.Errorf("expected[0].Line = %d, want 3", cmd.Expected[0].Line)
	}
}

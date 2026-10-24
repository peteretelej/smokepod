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

func TestParse_XFail(t *testing.T) {
	tf, err := Parse(testdataPath("xfail.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Sections) != 3 {
		t.Fatalf("sections = %d, want 3", len(tf.Sections))
	}

	// normal section: no xfail
	normal := requireSection(t, tf, "normal")
	if normal.XFail {
		t.Error("normal.XFail = true, want false")
	}
	if normal.XFailReason != "" {
		t.Errorf("normal.XFailReason = %q, want empty", normal.XFailReason)
	}

	// broken section: xfail without reason
	broken := requireSection(t, tf, "broken")
	if !broken.XFail {
		t.Error("broken.XFail = false, want true")
	}
	if broken.XFailReason != "" {
		t.Errorf("broken.XFailReason = %q, want empty", broken.XFailReason)
	}

	// bugged section: xfail with reason
	bugged := requireSection(t, tf, "bugged")
	if !bugged.XFail {
		t.Error("bugged.XFail = false, want true")
	}
	if bugged.XFailReason != "lexer bug #42" {
		t.Errorf("bugged.XFailReason = %q, want %q", bugged.XFailReason, "lexer bug #42")
	}
}

func TestParse_XFailPreservesName(t *testing.T) {
	tf, err := Parse(testdataPath("xfail.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"normal", "broken", "bugged"}
	for i, name := range tf.Order {
		if name != expected[i] {
			t.Errorf("order[%d] = %q, want %q", i, name, expected[i])
		}
	}

	for _, name := range expected {
		section := tf.GetSection(name)
		if section == nil {
			t.Errorf("section %q not found", name)
			continue
		}
		if section.Name != name {
			t.Errorf("section.Name = %q, want %q", section.Name, name)
		}
	}
}

func TestParse_Metadata(t *testing.T) {
	t.Run("target and target-arg before sections", func(t *testing.T) {
		f, err := os.CreateTemp("", "meta*.test")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(f.Name()) }()
		content := "# target: /usr/bin/jq\n# target-arg: --tab\n\n## basic\n$ echo hello\nhello\n"
		if _, err := f.WriteString(content); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()

		tf, err := Parse(f.Name())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tf.Metadata == nil {
			t.Fatal("Metadata is nil, want non-nil")
		}
		if got := tf.Metadata["target"]; len(got) != 1 || got[0] != "/usr/bin/jq" {
			t.Errorf("target = %v, want [/usr/bin/jq]", got)
		}
		if got := tf.Metadata["target-arg"]; len(got) != 1 || got[0] != "--tab" {
			t.Errorf("target-arg = %v, want [--tab]", got)
		}
	})

	t.Run("mode directive", func(t *testing.T) {
		f, err := os.CreateTemp("", "meta*.test")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(f.Name()) }()
		content := "# mode: process\n\n## basic\n$ echo hello\nhello\n"
		if _, err := f.WriteString(content); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()

		tf, err := Parse(f.Name())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tf.Metadata == nil {
			t.Fatal("Metadata is nil, want non-nil")
		}
		if got := tf.Metadata["mode"]; len(got) != 1 || got[0] != "process" {
			t.Errorf("mode = %v, want [process]", got)
		}
	})

	t.Run("regular comments before sections", func(t *testing.T) {
		f, err := os.CreateTemp("", "meta*.test")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(f.Name()) }()
		content := "# this is just a comment\n# another comment without colon pattern\n\n## basic\n$ echo hello\nhello\n"
		if _, err := f.WriteString(content); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()

		tf, err := Parse(f.Name())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tf.Metadata != nil {
			t.Errorf("Metadata = %v, want nil", tf.Metadata)
		}
	})

	t.Run("metadata-like comments after section header ignored", func(t *testing.T) {
		f, err := os.CreateTemp("", "meta*.test")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(f.Name()) }()
		content := "## basic\n# target: /bin/sh\n$ echo hello\nhello\n"
		if _, err := f.WriteString(content); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()

		tf, err := Parse(f.Name())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tf.Metadata != nil {
			t.Errorf("Metadata = %v, want nil (directives after section header should be ignored)", tf.Metadata)
		}
	})

	t.Run("no metadata at all", func(t *testing.T) {
		tf, err := Parse(testdataPath("simple.test"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tf.Metadata != nil {
			t.Errorf("Metadata = %v, want nil", tf.Metadata)
		}

		// sections should still parse normally
		if len(tf.Sections) != 1 {
			t.Errorf("sections = %d, want 1", len(tf.Sections))
		}
	})

	t.Run("multiple target-arg values collected in order", func(t *testing.T) {
		f, err := os.CreateTemp("", "meta*.test")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(f.Name()) }()
		content := "# target-arg: --tab\n# target-arg: --raw-output\n# target-arg: --indent\n\n## basic\n$ echo hello\nhello\n"
		if _, err := f.WriteString(content); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()

		tf, err := Parse(f.Name())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tf.Metadata == nil {
			t.Fatal("Metadata is nil, want non-nil")
		}
		got := tf.Metadata["target-arg"]
		want := []string{"--tab", "--raw-output", "--indent"}
		if len(got) != len(want) {
			t.Fatalf("target-arg = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("target-arg[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})
}

func writeTempTest(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "normalize*.test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestParse_NormalizeDirective(t *testing.T) {
	t.Run("single filter", func(t *testing.T) {
		path := writeTempTest(t, "# normalize: timestamps\n\n## basic\n$ echo hello\nhello\n")
		tf, err := Parse(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"timestamps"}
		if len(tf.Normalize) != len(want) || tf.Normalize[0] != want[0] {
			t.Errorf("Normalize = %v, want %v", tf.Normalize, want)
		}
	})

	t.Run("comma-separated filters with spaces", func(t *testing.T) {
		path := writeTempTest(t, "# normalize: timestamps, uuids , paths\n\n## basic\n$ echo hello\nhello\n")
		tf, err := Parse(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"timestamps", "uuids", "paths"}
		if len(tf.Normalize) != len(want) {
			t.Fatalf("Normalize = %v, want %v", tf.Normalize, want)
		}
		for i := range want {
			if tf.Normalize[i] != want[i] {
				t.Errorf("Normalize[%d] = %q, want %q", i, tf.Normalize[i], want[i])
			}
		}
	})

	t.Run("unknown filter is a parse error with suggestion", func(t *testing.T) {
		path := writeTempTest(t, "# normalize: timestamp\n\n## basic\n$ echo hello\nhello\n")
		_, err := Parse(path)
		if err == nil {
			t.Fatal("expected parse error for unknown filter, got nil")
		}
		if !strings.Contains(err.Error(), "unknown normalize filter") {
			t.Errorf("error = %q, want it to mention the unknown filter", err.Error())
		}
		if !strings.Contains(err.Error(), `"timestamps"`) {
			t.Errorf("error = %q, want a suggestion", err.Error())
		}
	})

	t.Run("empty filter is a parse error", func(t *testing.T) {
		path := writeTempTest(t, "# normalize: timestamps,,uuids\n\n## basic\n$ echo hello\nhello\n")
		_, err := Parse(path)
		if err == nil {
			t.Fatal("expected parse error for empty filter, got nil")
		}
		if !strings.Contains(err.Error(), "empty normalize filter") {
			t.Errorf("error = %q, want it to mention the empty filter", err.Error())
		}
	})
}

func TestParse_NormAndJSONSuffixes(t *testing.T) {
	path := writeTempTest(t, "## tiers\n$ echo one\nONE (norm)\n$ echo '{\"a\":1}'\n{\"a\":1} (json)\n$ echo err\n{} (json,stderr)\n")
	tf, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	section := requireSection(t, tf, "tiers")
	if len(section.Commands) != 3 {
		t.Fatalf("commands = %d, want 3", len(section.Commands))
	}

	norm := section.Commands[0].Expected[0]
	if !norm.IsNorm {
		t.Error("expected IsNorm=true for (norm)")
	}
	if norm.IsJSON || norm.IsRegex {
		t.Error("(norm) should not set IsJSON or IsRegex")
	}
	if norm.Text != "ONE" {
		t.Errorf("text = %q, want %q", norm.Text, "ONE")
	}

	js := section.Commands[1].Expected[0]
	if !js.IsJSON {
		t.Error("expected IsJSON=true for (json)")
	}
	if js.IsNorm {
		t.Error("(json) should not set IsNorm")
	}

	combined := section.Commands[2].Expected[0]
	if !combined.IsJSON || !combined.IsStderr {
		t.Errorf("(json,stderr) should set both, got json=%v stderr=%v", combined.IsJSON, combined.IsStderr)
	}
}

func TestParse_XFailLineNumbers(t *testing.T) {
	tf, err := Parse(testdataPath("xfail.test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		line int
	}{
		{"normal", 1},
		{"broken", 5},
		{"bugged", 9},
	}

	for _, tt := range tests {
		section := requireSection(t, tf, tt.name)
		if section.Line != tt.line {
			t.Errorf("section %q: Line = %d, want %d", tt.name, section.Line, tt.line)
		}
	}
}

func TestParse_MisplacedDirectiveWarning(t *testing.T) {
	f, err := os.CreateTemp("", "misplaced*.test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	content := "## section\n# target: /bin/sh\n$ echo hi\nhi\n"
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	tf, err := Parse(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want 1 warning", tf.Warnings)
	}
	if !strings.Contains(tf.Warnings[0], "will be ignored") {
		t.Errorf("warning = %q, want to contain 'will be ignored'", tf.Warnings[0])
	}
	// Misplaced directive should NOT be stored in Metadata
	if tf.Metadata != nil {
		t.Errorf("Metadata = %v, want nil (misplaced directive should not be stored)", tf.Metadata)
	}
}

func TestParse_UnknownDirectiveWithSuggestion(t *testing.T) {
	f, err := os.CreateTemp("", "unknown*.test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	content := "# taret: /bin/sh\n## s\n$ echo hi\nhi\n"
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	tf, err := Parse(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want 1 warning", tf.Warnings)
	}
	if !strings.Contains(tf.Warnings[0], `did you mean "target"`) {
		t.Errorf("warning = %q, want to contain 'did you mean \"target\"'", tf.Warnings[0])
	}
	// Unknown directive should still be stored in Metadata
	if tf.Metadata == nil || tf.Metadata["taret"] == nil {
		t.Error("unknown directive should still be stored in Metadata")
	}
}

func TestParse_UnknownDirectiveWithoutSuggestion(t *testing.T) {
	f, err := os.CreateTemp("", "unknown*.test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	content := "# zzzzz: foo\n## s\n$ echo hi\nhi\n"
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	tf, err := Parse(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want 1 warning", tf.Warnings)
	}
	if !strings.Contains(tf.Warnings[0], `unknown directive "zzzzz"`) {
		t.Errorf("warning = %q, want to contain 'unknown directive \"zzzzz\"'", tf.Warnings[0])
	}
	if strings.Contains(tf.Warnings[0], "did you mean") {
		t.Errorf("warning = %q, should NOT contain 'did you mean' for distant key", tf.Warnings[0])
	}
	// Still stored in Metadata
	if tf.Metadata == nil || tf.Metadata["zzzzz"] == nil {
		t.Error("unknown directive should still be stored in Metadata")
	}
}

func TestParse_KnownDirectivesNoWarnings(t *testing.T) {
	f, err := os.CreateTemp("", "known*.test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	content := "# target: /bin/sh\n# mode: shell\n## s\n$ echo hi\nhi\n"
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	tf, err := Parse(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Warnings) != 0 {
		t.Errorf("Warnings = %v, want no warnings for known directives", tf.Warnings)
	}
}

func TestEditDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"taret", "target", 1},
		{"mode", "mode", 0},
		{"zzzzz", "target", 6},
		{"", "target", 6},
		{"target", "", 6},
		{"target-arg", "target-arg", 0},
	}
	for _, tt := range tests {
		got := editDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

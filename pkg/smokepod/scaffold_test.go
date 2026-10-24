package smokepod

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplatizeVersion(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"stub version 1.2.3", `stub version \d+\.\d+\.\d+`},
		{"v2.10.0-rc.1", `v\d+\.\d+\.\d+-rc\.\d+`},
		{"dev", "dev"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := templatizeVersion(tt.in); got != tt.want {
			t.Errorf("templatizeVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestScaffoldSectionName(t *testing.T) {
	used := map[string]bool{}
	first := scaffoldSectionName("list", used)
	if first != "list" {
		t.Errorf("name = %q, want list", first)
	}
	if second := scaffoldSectionName("list", used); second != "list-2" {
		t.Errorf("repeat name = %q, want list-2", second)
	}
	if got := scaffoldSectionName("config show!", used); got != "config-show" {
		t.Errorf("sanitized name = %q, want config-show", got)
	}
	if got := scaffoldSectionName("***", used); got != "probe" {
		t.Errorf("name for symbols = %q, want probe", got)
	}
}

func TestRenderScaffold(t *testing.T) {
	sections := []scaffoldSection{
		{name: "version", command: `"SMOKEPOD..." --version`, expected: []string{`stub version \d+\.\d+\.\d+ (re)`}, active: false},
		{name: "help", command: `"SMOKEPOD..." --help`, expected: []string{"usage: stub [command]"}, active: true},
	}
	out := renderScaffold(sections)

	if !strings.Contains(out, "# normalize: timestamps,paths") {
		t.Errorf("scaffold missing normalize directive:\n%s", out)
	}
	helpIdx := strings.Index(out, "## help")
	failIdx := strings.Index(out, "# ## version")
	if helpIdx == -1 || failIdx == -1 {
		t.Fatalf("scaffold missing sections:\n%s", out)
	}
	if helpIdx > failIdx {
		t.Errorf("active sections must come before commented ones:\n%s", out)
	}
	if !strings.Contains(out, "# $ \"SMOKEPOD...\" --version") {
		t.Errorf("commented section not fully commented:\n%s", out)
	}
}

func TestInit_NoTarget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tests")

	res, err := Init(t.Context(), InitOptions{Dir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, ScaffoldFileName))
	if err != nil {
		t.Fatalf("scaffold not written: %v", err)
	}
	if !strings.Contains(string(content), "# normalize: timestamps,paths") {
		t.Errorf("scaffold missing normalize directive:\n%s", content)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "## ") {
			t.Errorf("no-target scaffold has an active section %q, want all commented", line)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), InitFixturesDir)); !os.IsNotExist(err) {
		t.Errorf("fixtures written for no-target init, want none")
	}
	if len(res.Files) != 1 || res.Files[0] != filepath.Join(dir, ScaffoldFileName) {
		t.Errorf("res.Files = %v, want only the scaffold", res.Files)
	}
}

func TestInit_RefusesExistingWithoutForce(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "tests")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	testPath := filepath.Join(dir, ScaffoldFileName)
	if err := os.WriteFile(testPath, []byte("## mine\n$ echo kept\nkept\n"), 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Init(t.Context(), InitOptions{Dir: dir})
	if err == nil {
		t.Fatal("expected ErrScaffoldExists, got nil")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("err = %v, want --force hint", err)
	}
	after, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("existing .test file was modified by refused init")
	}

	if _, err := Init(t.Context(), InitOptions{Dir: dir, Force: true}); err != nil {
		t.Errorf("init --force failed: %v", err)
	}
	overwritten, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(overwritten), "## mine") {
		t.Error("--force did not overwrite the scaffolded file")
	}
}

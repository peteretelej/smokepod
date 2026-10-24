package smokepod

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// silenceRecordOutput swallows Record's stderr progress while fn runs.
func silenceRecordOutput(t *testing.T, fn func()) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, r)
		close(done)
	}()
	defer func() {
		_ = w.Close()
		os.Stderr = old
		<-done
		_ = r.Close()
	}()
	fn()
}

func TestRecord_WritesFixtureAndManifest(t *testing.T) {
	tmp := t.TempDir()
	testsDir := filepath.Join(tmp, "tests")
	fixturesDir := filepath.Join(tmp, "fixtures")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatal(err)
	}
	testPath := filepath.Join(testsDir, "hi.test")
	if err := os.WriteFile(testPath, []byte("## hi\n$ echo hi\nhi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var summary *RecordSummary
	var err error
	silenceRecordOutput(t, func() {
		summary, err = Record(t.Context(), RecordOptions{
			Target:      "echo",
			TestsDir:    testsDir,
			FixturesDir: fixturesDir,
			Indent:      "  ",
		})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Recorded != 1 || summary.Failed != 0 {
		t.Errorf("summary = %+v, want 1 recorded, 0 failed", summary)
	}

	fixture, err := ReadFixture(filepath.Join(fixturesDir, "hi.fixture.json"))
	if err != nil {
		t.Fatalf("fixture not written: %v", err)
	}
	cmds, ok := fixture.Sections["hi"]
	if !ok || len(cmds) != 1 || cmds[0].Stdout != "hi\n" {
		t.Errorf("fixture sections = %+v, want hi with stdout \"hi\\n\"", fixture.Sections)
	}

	manifest, err := LoadManifest(fixturesDir)
	if err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	if _, ok := manifest.Tests["hi.test"]; !ok {
		t.Errorf("manifest missing tests key hi.test: %+v", manifest.Tests)
	}
	if _, ok := manifest.Fixtures["hi.fixture.json"]; !ok {
		t.Errorf("manifest missing fixtures key hi.fixture.json: %+v", manifest.Fixtures)
	}
}

func TestRecord_EmptyTestsDir(t *testing.T) {
	tmp := t.TempDir()
	testsDir := filepath.Join(tmp, "tests")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Record(t.Context(), RecordOptions{TestsDir: testsDir, FixturesDir: filepath.Join(tmp, "fixtures")})
	if !errors.Is(err, ErrNoTestFiles) {
		t.Errorf("err = %v, want ErrNoTestFiles", err)
	}

	summary, err := Record(t.Context(), RecordOptions{TestsDir: testsDir, FixturesDir: filepath.Join(tmp, "fixtures"), AllowEmpty: true})
	if err != nil {
		t.Fatalf("unexpected error with AllowEmpty: %v", err)
	}
	if summary.Recorded != 0 {
		t.Errorf("summary = %+v, want nothing recorded", summary)
	}
	if _, err := os.Stat(ManifestPath(filepath.Join(tmp, "fixtures"))); !os.IsNotExist(err) {
		t.Errorf("manifest written for empty run, want none")
	}
}

func TestRecord_SkipsExistingFixtureWithoutUpdate(t *testing.T) {
	tmp := t.TempDir()
	testsDir := filepath.Join(tmp, "tests")
	fixturesDir := filepath.Join(tmp, "fixtures")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "hi.test"), []byte("## hi\n$ echo hi\nhi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	opts := RecordOptions{Target: "echo", TestsDir: testsDir, FixturesDir: fixturesDir, Indent: "  "}

	silenceRecordOutput(t, func() {
		_, _ = Record(t.Context(), opts)
	})
	var summary *RecordSummary
	var err error
	silenceRecordOutput(t, func() {
		summary, err = Record(t.Context(), opts)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Skipped != 1 || summary.Recorded != 0 {
		t.Errorf("summary = %+v, want 1 skipped, 0 recorded", summary)
	}

	opts.Update = true
	silenceRecordOutput(t, func() {
		summary, err = Record(t.Context(), opts)
	})
	if err != nil {
		t.Fatalf("unexpected error with Update: %v", err)
	}
	if summary.Unchanged != 1 {
		t.Errorf("summary = %+v, want 1 unchanged with identical output", summary)
	}
}

func TestResolveTarget_Table(t *testing.T) {
	tests := []struct {
		name      string
		metadata  map[string][]string
		cliTarget string
		wantErr   string
	}{
		{"cli wins", map[string][]string{"target": {"/bin/sh"}}, "/bin/bash", ""},
		{"file fallback", map[string][]string{"target": {"/bin/sh"}}, "", ""},
		{"no target", nil, "", "no target"},
		{"invalid mode", map[string][]string{"target": {"/bin/sh"}, "mode": {"banana"}}, "", "invalid mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := ResolveTarget("t.test", tt.metadata, tt.cliTarget, nil, "")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("err = %v, want to contain %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

package smokepod

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	fixturesDir := t.TempDir()

	manifest := &Manifest{
		Tests:    map[string]string{"basics.test": "aaa"},
		Fixtures: map[string]string{"basics.fixture.json": "bbb"},
	}
	if err := WriteManifest(fixturesDir, manifest); err != nil {
		t.Fatalf("WriteManifest failed: %v", err)
	}

	loaded, err := LoadManifest(fixturesDir)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if loaded.Tests["basics.test"] != "aaa" {
		t.Errorf("Tests = %v, want basics.test entry", loaded.Tests)
	}
	if loaded.Fixtures["basics.fixture.json"] != "bbb" {
		t.Errorf("Fixtures = %v, want basics.fixture.json entry", loaded.Fixtures)
	}
}

func TestLoadManifest_Missing(t *testing.T) {
	_, err := LoadManifest(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error = %v, want fs.ErrNotExist", err)
	}
}

func TestWriteManifest_NilMaps(t *testing.T) {
	fixturesDir := t.TempDir()

	if err := WriteManifest(fixturesDir, &Manifest{}); err != nil {
		t.Fatalf("WriteManifest failed: %v", err)
	}

	loaded, err := LoadManifest(fixturesDir)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if loaded.Tests == nil || loaded.Fixtures == nil {
		t.Errorf("loaded maps should be non-nil, got %v / %v", loaded.Tests, loaded.Fixtures)
	}
}

func TestScanManifest_HashesTestsAndFixtures(t *testing.T) {
	tmpDir := t.TempDir()
	testsDir := filepath.Join(tmpDir, "tests")
	fixturesDir := filepath.Join(tmpDir, "fixtures")

	writeFile(t, filepath.Join(testsDir, "basics.test"), "## foo\n")
	writeFile(t, filepath.Join(testsDir, "sub", "nested.test"), "## bar\n")
	writeFile(t, filepath.Join(fixturesDir, "basics.fixture.json"), "{}")
	writeFile(t, filepath.Join(fixturesDir, ManifestFileName), "{}")

	manifest, err := ScanManifest(testsDir, fixturesDir)
	if err != nil {
		t.Fatalf("ScanManifest failed: %v", err)
	}

	if len(manifest.Tests) != 2 {
		t.Fatalf("len(Tests) = %d, want 2", len(manifest.Tests))
	}
	if _, ok := manifest.Tests["sub/nested.test"]; !ok {
		t.Errorf("Tests = %v, want slash-relative nested key", manifest.Tests)
	}
	// The manifest itself must never be hashed as a fixture.
	if len(manifest.Fixtures) != 1 {
		t.Fatalf("len(Fixtures) = %d, want 1 (manifest excluded)", len(manifest.Fixtures))
	}

	sum, err := HashFile(filepath.Join(testsDir, "basics.test"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Tests["basics.test"] != sum {
		t.Errorf("Tests[basics.test] = %q, want %q", manifest.Tests["basics.test"], sum)
	}
}

func TestScanManifest_SingleTestFile(t *testing.T) {
	tmpDir := t.TempDir()
	testsDir := filepath.Join(tmpDir, "tests")
	fixturesDir := filepath.Join(tmpDir, "fixtures")

	writeFile(t, filepath.Join(testsDir, "only.test"), "## foo\n")
	writeFile(t, filepath.Join(testsDir, "other.test"), "## skipped\n")
	writeFile(t, filepath.Join(fixturesDir, "only.fixture.json"), "{}")

	manifest, err := ScanManifest(filepath.Join(testsDir, "only.test"), fixturesDir)
	if err != nil {
		t.Fatalf("ScanManifest failed: %v", err)
	}

	if len(manifest.Tests) != 1 {
		t.Fatalf("len(Tests) = %d, want 1", len(manifest.Tests))
	}
	if _, ok := manifest.Tests["only.test"]; !ok {
		t.Errorf("Tests = %v, want only the named file", manifest.Tests)
	}
}

func TestDiffManifest_DriftKinds(t *testing.T) {
	tmpDir := t.TempDir()
	testsDir := filepath.Join(tmpDir, "tests")
	fixturesDir := filepath.Join(tmpDir, "fixtures")

	writeFile(t, filepath.Join(testsDir, "a.test"), "a")
	writeFile(t, filepath.Join(testsDir, "gone.test"), "gone")
	writeFile(t, filepath.Join(fixturesDir, "a.fixture.json"), "a")
	writeFile(t, filepath.Join(fixturesDir, "gone.fixture.json"), "gone")

	recorded, err := ScanManifest(testsDir, fixturesDir)
	if err != nil {
		t.Fatalf("ScanManifest failed: %v", err)
	}

	if drift := DiffManifest(recorded, recorded); len(drift) != 0 {
		t.Fatalf("DiffManifest against itself = %v, want empty", drift)
	}

	// Modify a.test, delete gone.test, add new.test and new.fixture.json.
	writeFile(t, filepath.Join(testsDir, "a.test"), "changed")
	if err := os.Remove(filepath.Join(testsDir, "gone.test")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(testsDir, "new.test"), "new")
	writeFile(t, filepath.Join(fixturesDir, "new.fixture.json"), "new")

	onDisk, err := ScanManifest(testsDir, fixturesDir)
	if err != nil {
		t.Fatalf("ScanManifest failed: %v", err)
	}

	drift := DiffManifest(recorded, onDisk)
	want := []Drift{
		{Tree: "fixtures", Path: "new.fixture.json", Kind: "untracked"},
		{Tree: "tests", Path: "a.test", Kind: "modified"},
		{Tree: "tests", Path: "gone.test", Kind: "missing"},
		{Tree: "tests", Path: "new.test", Kind: "untracked"},
	}
	if len(drift) != len(want) {
		t.Fatalf("drift = %v, want %v", drift, want)
	}
	for i, d := range drift {
		if d != want[i] {
			t.Errorf("drift[%d] = %+v, want %+v", i, d, want[i])
		}
	}
}

func TestFixtureKeyForTest(t *testing.T) {
	tests := []struct {
		testKey string
		want    string
	}{
		{"basics.test", "basics.fixture.json"},
		{"sub/dir/pipes.test", "sub/dir/pipes.fixture.json"},
	}
	for _, tc := range tests {
		if got := FixtureKeyForTest(tc.testKey); got != tc.want {
			t.Errorf("FixtureKeyForTest(%q) = %q, want %q", tc.testKey, got, tc.want)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

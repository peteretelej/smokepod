package smokepod

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ManifestFileName is the manifest file stored inside the fixtures directory.
const ManifestFileName = "manifest.json"

// Manifest records SHA-256 hashes of the recorded baseline so verify can
// detect fixtures or test files that changed after record. Tests keys are
// relative to the tests directory; fixtures keys are relative to the fixtures
// directory. Keys always use forward slashes.
type Manifest struct {
	Tests    map[string]string `json:"tests"`
	Fixtures map[string]string `json:"fixtures"`
}

// Drift describes one baseline file that changed after record.
type Drift struct {
	Tree string `json:"tree"` // "tests" or "fixtures"
	Path string `json:"path"` // manifest key (slash-separated, relative to the tree root)
	Kind string `json:"kind"` // "modified", "missing", or "untracked"
}

// ManifestPath returns the manifest location inside the fixtures directory.
func ManifestPath(fixturesDir string) string {
	return filepath.Join(fixturesDir, ManifestFileName)
}

// LoadManifest reads the manifest from the fixtures directory. The returned
// error wraps fs.ErrNotExist when no baseline has been recorded yet.
func LoadManifest(fixturesDir string) (*Manifest, error) {
	data, err := os.ReadFile(ManifestPath(fixturesDir))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	if manifest.Tests == nil {
		manifest.Tests = map[string]string{}
	}
	if manifest.Fixtures == nil {
		manifest.Fixtures = map[string]string{}
	}

	return &manifest, nil
}

// WriteManifest persists the manifest inside the fixtures directory.
func WriteManifest(fixturesDir string, manifest *Manifest) error {
	if manifest.Tests == nil {
		manifest.Tests = map[string]string{}
	}
	if manifest.Fixtures == nil {
		manifest.Fixtures = map[string]string{}
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(fixturesDir, 0755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}
	if err := os.WriteFile(ManifestPath(fixturesDir), data, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}

// ScanManifest hashes the .test and fixture files currently on disk,
// producing the counterpart that DiffManifest compares against a recorded
// manifest.
func ScanManifest(testsDir, fixturesDir string) (*Manifest, error) {
	manifest := &Manifest{Tests: map[string]string{}, Fixtures: map[string]string{}}

	if info, err := os.Stat(testsDir); err == nil && !info.IsDir() {
		// Single test file: baseline scope is that file only, keyed relative
		// to its parent directory (matches FixturePathFromTest).
		sum, err := HashFile(testsDir)
		if err != nil {
			return nil, err
		}
		key, err := filepath.Rel(filepath.Dir(testsDir), testsDir)
		if err != nil {
			return nil, fmt.Errorf("scanning tests: %w", err)
		}
		manifest.Tests[filepath.ToSlash(key)] = sum
	} else {
		tests, err := scanTree(testsDir, ".test")
		if err != nil {
			return nil, fmt.Errorf("scanning tests: %w", err)
		}
		manifest.Tests = tests
	}

	fixtures, err := scanTree(fixturesDir, ".fixture.json")
	if err != nil {
		return nil, fmt.Errorf("scanning fixtures: %w", err)
	}
	manifest.Fixtures = fixtures

	return manifest, nil
}

// DiffManifest compares a recorded manifest against the on-disk state from
// ScanManifest and returns the drift entries sorted by tree, path, kind.
func DiffManifest(recorded, onDisk *Manifest) []Drift {
	var drift []Drift
	drift = append(drift, diffTree("tests", recorded.Tests, onDisk.Tests)...)
	drift = append(drift, diffTree("fixtures", recorded.Fixtures, onDisk.Fixtures)...)

	slices.SortFunc(drift, func(a, b Drift) int {
		if c := strings.Compare(a.Tree, b.Tree); c != 0 {
			return c
		}
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return strings.Compare(a.Kind, b.Kind)
	})
	return drift
}

// FixtureKeyForTest maps a tests-tree key to its fixtures-tree key using the
// .test -> .fixture.json convention shared with FixturePathFromTest.
func FixtureKeyForTest(testKey string) string {
	return strings.TrimSuffix(testKey, ".test") + ".fixture.json"
}

// HashFile returns the hex-encoded SHA-256 of the file contents.
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("hashing %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func scanTree(root, suffix string) (map[string]string, error) {
	hashes := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, suffix) {
			return nil
		}
		sum, err := HashFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(rel)] = sum
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hashes, nil
}

func diffTree(tree string, recorded, onDisk map[string]string) []Drift {
	var drift []Drift
	for path, sum := range recorded {
		actual, ok := onDisk[path]
		switch {
		case !ok:
			drift = append(drift, Drift{Tree: tree, Path: path, Kind: "missing"})
		case actual != sum:
			drift = append(drift, Drift{Tree: tree, Path: path, Kind: "modified"})
		}
	}
	for path := range onDisk {
		if _, ok := recorded[path]; !ok {
			drift = append(drift, Drift{Tree: tree, Path: path, Kind: "untracked"})
		}
	}
	return drift
}

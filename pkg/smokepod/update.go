package smokepod

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peteretelej/smokepod/internal/testfile"
)

// ReRecordError reports that some test files failed to re-record; nothing
// was changed. Error text is the user-facing message.
type ReRecordError struct {
	Failed int
}

func (e *ReRecordError) Error() string {
	return fmt.Sprintf("%d test file(s) failed to re-record; nothing was changed", e.Failed)
}

// UpdateOptions configures one update planning run.
type UpdateOptions struct {
	Target      string
	TargetArgs  []string
	Mode        string
	TestsDir    string
	FixturesDir string
	// All re-records every fixture, not only drifted ones.
	All bool
	// Indent is the JSON indentation for rewritten fixtures.
	Indent string
}

// FixtureUpdate is one planned fixture rewrite: the old fixture on disk and
// the fixture re-recording produced.
type FixtureUpdate struct {
	FixturePath string
	Old         *FixtureFile
	New         *FixtureFile
}

// UpdatePlan is the outcome of planning an update: the fixtures to rewrite
// (re-recorded, not yet written) and the directories the plan belongs to.
// Synced reports that the baseline is already in sync and there is nothing
// to do.
type UpdatePlan struct {
	Updates     []FixtureUpdate
	TestsDir    string
	FixturesDir string
	Indent      string
	Synced      bool
}

// PlanUpdate re-records the drifted fixtures and renders the old-vs-new
// changes without writing anything. Apply commits the plan. The caller owns
// cancellation (context) and timeout.
func PlanUpdate(ctx context.Context, opts UpdateOptions) (*UpdatePlan, error) {
	recorded, err := LoadManifest(opts.FixturesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, &UsageError{fmt.Errorf("no %s in %s; nothing to update - run `smokepod record` once to create a protected baseline", ManifestFileName, opts.FixturesDir)}
		}
		return nil, fmt.Errorf("Error reading manifest: %w", err)
	}

	testFiles, err := FindTestFiles(opts.TestsDir)
	if err != nil {
		return nil, &UsageError{fmt.Errorf("Error finding test files: %v", err)}
	}
	if len(testFiles) == 0 {
		return nil, &UsageError{fmt.Errorf("no .test files found in %s", opts.TestsDir)}
	}

	onDisk, err := ScanManifest(opts.TestsDir, opts.FixturesDir)
	if err != nil {
		return nil, fmt.Errorf("Error scanning baseline: %w", err)
	}
	drift := DiffManifest(recorded, onDisk)

	// Even with an empty re-record scope (e.g. a deleted .test file), drift
	// still needs the manifest refresh below to be accepted.
	scope := updateScope(testFiles, opts.TestsDir, drift, opts.All)
	if len(drift) == 0 && !opts.All {
		return &UpdatePlan{Synced: true}, nil
	}

	// Create shared ProcessTarget when the CLI provides both --mode process
	// and --target
	var sharedProcTarget *ProcessTarget
	if opts.Mode == "process" && opts.Target != "" {
		if _, err := exec.LookPath(opts.Target); err != nil {
			return nil, &UsageError{fmt.Errorf("target %q not found in PATH", opts.Target)}
		}
		pt, err := NewProcessTarget(ctx, opts.Target, opts.TargetArgs...)
		if err != nil {
			return nil, fmt.Errorf("Error creating shared process target: %w", err)
		}
		sharedProcTarget = pt
		defer func() { _ = sharedProcTarget.Close() }()
	}

	var updates []FixtureUpdate
	failed := 0
	for _, testFile := range scope {
		newFixture, err := reRecordFixture(ctx, testFile, opts.Target, opts.TargetArgs, opts.Mode, sharedProcTarget)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error re-recording %s: %v\n", testFile, err)
			failed++
			continue
		}
		fixturePath := FixturePathFromTest(testFile, opts.TestsDir, opts.FixturesDir)
		oldFixture, err := ReadFixture(fixturePath)
		if err != nil {
			oldFixture = nil
		}
		updates = append(updates, FixtureUpdate{FixturePath: fixturePath, Old: oldFixture, New: newFixture})
	}
	if failed > 0 {
		return nil, &ReRecordError{Failed: failed}
	}

	return &UpdatePlan{
		Updates:     updates,
		TestsDir:    opts.TestsDir,
		FixturesDir: opts.FixturesDir,
		Indent:      opts.Indent,
	}, nil
}

// Apply writes the planned fixtures and refreshes the manifest. It returns
// the number of fixture files actually written.
func (p *UpdatePlan) Apply() (int, error) {
	written := 0
	for _, u := range p.Updates {
		didWrite, err := WriteFixture(u.FixturePath, u.New, p.Indent)
		if err != nil {
			return written, fmt.Errorf("Error writing fixture %s: %w", u.FixturePath, err)
		}
		if didWrite {
			written++
		}
	}

	manifest, err := ScanManifest(p.TestsDir, p.FixturesDir)
	if err != nil {
		return written, fmt.Errorf("Error scanning baseline: %w", err)
	}
	if err := WriteManifest(p.FixturesDir, manifest); err != nil {
		return written, fmt.Errorf("Error writing manifest: %w", err)
	}
	return written, nil
}

// RenderDiff renders the plan as old-vs-new fixture content per file,
// showing only the commands and values that differ.
func (p *UpdatePlan) RenderDiff() string {
	var out strings.Builder
	for _, u := range p.Updates {
		fmt.Fprintf(&out, "--- recorded: %s\n+++ re-recorded\n", u.FixturePath)
		diff := renderFixtureDiff(u.Old, u.New)
		if diff == "" {
			out.WriteString("no output changes\n")
		} else {
			out.WriteString(diff)
		}
		out.WriteString("\n")
	}
	return out.String()
}

// updateScope picks the test files to re-record: everything with --all,
// otherwise the files behind drifted manifest entries (a drifted test file
// implicates its fixture and vice versa).
func updateScope(testFiles []string, testsPath string, drift []Drift, all bool) []string {
	if all {
		return testFiles
	}

	driftedTests := map[string]bool{}
	driftedFixtures := map[string]bool{}
	for _, d := range drift {
		if d.Tree == "tests" {
			driftedTests[d.Path] = true
		} else {
			driftedFixtures[d.Path] = true
		}
	}

	testsRoot := testsPath
	if info, err := os.Stat(testsPath); err == nil && !info.IsDir() {
		testsRoot = filepath.Dir(testsPath)
	}

	var scope []string
	for _, testFile := range testFiles {
		rel, err := filepath.Rel(testsRoot, testFile)
		if err != nil {
			continue
		}
		key := filepath.ToSlash(rel)
		if driftedTests[key] || driftedFixtures[FixtureKeyForTest(key)] {
			scope = append(scope, testFile)
		}
	}
	return scope
}

// reRecordFixture runs a test file's commands and builds the fixture that
// record would write, without writing it.
func reRecordFixture(ctx context.Context, testFile, cliTarget string, cliTargetArgs []string, cliMode string, sharedProcTarget *ProcessTarget) (*FixtureFile, error) {
	tf, err := testfile.Parse(testFile)
	if err != nil {
		return nil, err
	}

	resolvedTarget, resolvedArgs, resolvedMode, err := ResolveTarget(testFile, tf.Metadata, cliTarget, cliTargetArgs, cliMode)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath(resolvedTarget); err != nil {
		return nil, fmt.Errorf("target %q not found in PATH", resolvedTarget)
	}

	var targetExec Target
	if resolvedMode == "process" {
		if sharedProcTarget != nil {
			targetExec = sharedProcTarget
		} else {
			procTarget, err := NewProcessTarget(ctx, resolvedTarget, resolvedArgs...)
			if err != nil {
				return nil, err
			}
			targetExec = procTarget
		}
	} else {
		targetExec = NewLocalTarget(resolvedTarget, resolvedArgs, nil, resolvedMode)
	}
	defer func() {
		if targetExec != nil && targetExec != sharedProcTarget {
			_ = targetExec.Close()
		}
	}()

	platform := DetectPlatform(ctx, targetExec)

	sections, err := tf.GetSections(nil)
	if err != nil {
		return nil, err
	}

	fixture := &FixtureFile{
		Source:           testFile,
		RecordedWith:     resolvedTarget,
		RecordedWithArgs: resolvedArgs,
		Platform:         platform,
		Sections:         make(map[string][]FixtureCommand),
	}
	for _, section := range sections {
		commands := []FixtureCommand{}
		for _, tc := range section.Commands {
			result, err := targetExec.Exec(ctx, tc.Cmd)
			if err != nil {
				return nil, fmt.Errorf("executing %q: %w", tc.Cmd, err)
			}
			commands = append(commands, FixtureCommand{
				Line:     tc.Line,
				Command:  tc.Cmd,
				Stdout:   result.Stdout,
				Stderr:   result.Stderr,
				ExitCode: result.ExitCode,
			})
		}
		fixture.Sections[section.Name] = commands
	}
	return fixture, nil
}

// renderFixtureDiff renders old-vs-new fixture content per section, showing
// only the commands and values that differ. Empty result means no changes.
func renderFixtureDiff(oldFixture, newFixture *FixtureFile) string {
	sectionNames := map[string]bool{}
	for name := range oldFixture.Sections {
		sectionNames[name] = true
	}
	for name := range newFixture.Sections {
		sectionNames[name] = true
	}
	sorted := make([]string, 0, len(sectionNames))
	for name := range sectionNames {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	var out strings.Builder
	for _, name := range sorted {
		var section strings.Builder
		oldCmds := oldFixture.Sections[name]
		newCmds := newFixture.Sections[name]
		count := len(oldCmds)
		if len(newCmds) > count {
			count = len(newCmds)
		}
		for i := 0; i < count; i++ {
			var oc, nc *FixtureCommand
			if i < len(oldCmds) {
				oc = &oldCmds[i]
			}
			if i < len(newCmds) {
				nc = &newCmds[i]
			}
			writeCommandDiff(&section, oc, nc)
		}
		if section.Len() > 0 {
			fmt.Fprintf(&out, "## %s\n", name)
			out.WriteString(section.String())
		}
	}
	return out.String()
}

func writeCommandDiff(b *strings.Builder, oc, nc *FixtureCommand) {
	switch {
	case oc != nil && nc != nil:
		if oc.Command != nc.Command {
			fmt.Fprintf(b, "- $ %s\n+ $ %s\n", oc.Command, nc.Command)
		} else {
			fmt.Fprintf(b, "$ %s\n", oc.Command)
		}
		writeValueDiff(b, oc.Stdout, nc.Stdout)
		writeValueDiff(b, oc.Stderr, nc.Stderr)
		if oc.ExitCode != nc.ExitCode {
			fmt.Fprintf(b, "exit code: %d -> %d\n", oc.ExitCode, nc.ExitCode)
		}
	case oc != nil:
		fmt.Fprintf(b, "- $ %s\n", oc.Command)
		writeLines(b, "-", oc.Stdout)
	default:
		fmt.Fprintf(b, "+ $ %s\n", nc.Command)
		writeLines(b, "+", nc.Stdout)
	}
}

func writeValueDiff(b *strings.Builder, oldValue, newValue string) {
	if oldValue == newValue {
		return
	}
	writeLines(b, "-", oldValue)
	writeLines(b, "+", newValue)
}

func writeLines(b *strings.Builder, sign, value string) {
	trimmed := strings.TrimSuffix(value, "\n")
	if trimmed == "" {
		return
	}
	for _, line := range strings.Split(trimmed, "\n") {
		fmt.Fprintf(b, "%s %s\n", sign, line)
	}
}

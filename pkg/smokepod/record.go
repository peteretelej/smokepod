package smokepod

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/peteretelej/smokepod/internal/testfile"
)

// Record flow errors that callers classify into exit codes: discovery
// failures are configuration problems, everything else is runtime.
var (
	// ErrNoTestFiles is returned when discovery finds no .test files and
	// RecordOptions.AllowEmpty is false.
	ErrNoTestFiles = errors.New("no .test files found")

	// ErrDiscovery marks test-discovery failures.
	ErrDiscovery = errors.New("test discovery failed")
)

// RecordOptions configures one record run.
type RecordOptions struct {
	Target      string
	TargetArgs  []string
	Mode        string
	TestsDir    string
	FixturesDir string
	Update      bool
	RunSections []string
	AllowEmpty  bool
	Indent      string
}

// RecordSummary counts what one record run did per test file.
type RecordSummary struct {
	Recorded  int
	Unchanged int
	Skipped   int
	Failed    int
	// FixturePaths lists the fixture files written during this run
	// (paths for Recorded files only).
	FixturePaths []string
}

// Record runs the commands from every discovered .test file against the
// target and writes fixtures plus the manifest that protects them. The
// caller owns cancellation (context), timeout, and signal handling; progress
// is reported on stderr.
func Record(ctx context.Context, opts RecordOptions) (*RecordSummary, error) {
	testFiles, err := FindTestFiles(opts.TestsDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDiscovery, err)
	}

	summary := &RecordSummary{}
	if len(testFiles) == 0 {
		if !opts.AllowEmpty {
			return nil, fmt.Errorf("%w in %s; use --allow-empty to allow", ErrNoTestFiles, opts.TestsDir)
		}
		return summary, nil
	}

	// Create shared ProcessTarget when the CLI provides both --mode process
	// and --target
	var sharedProcTarget *ProcessTarget
	if opts.Mode == "process" && opts.Target != "" {
		if _, err := exec.LookPath(opts.Target); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrTargetNotFound, opts.Target)
		}
		pt, err := NewProcessTarget(ctx, opts.Target, opts.TargetArgs...)
		if err != nil {
			return nil, fmt.Errorf("creating shared process target: %w", err)
		}
		sharedProcTarget = pt
		defer func() { _ = sharedProcTarget.Close() }()
	}

	closeTarget := func(t Target) {
		if t != nil && t != sharedProcTarget {
			_ = t.Close()
		}
	}

	for _, testFile := range testFiles {
		fixturePath := FixturePathFromTest(testFile, opts.TestsDir, opts.FixturesDir)

		if !opts.Update {
			if _, err := os.Stat(fixturePath); err == nil {
				fmt.Fprintf(os.Stderr, "Skipping %s (fixture exists, use --update to overwrite)\n", testFile)
				summary.Skipped++
				continue
			}
		}

		tf, err := testfile.Parse(testFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", testFile, err)
			summary.Failed++
			continue
		}

		for _, w := range tf.Warnings {
			fmt.Fprintf(os.Stderr, "Warning: %s: %s\n", testFile, w)
		}

		resolvedTarget, resolvedArgs, resolvedMode, err := ResolveTarget(testFile, tf.Metadata, opts.Target, opts.TargetArgs, opts.Mode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			summary.Failed++
			continue
		}

		if _, err := exec.LookPath(resolvedTarget); err != nil {
			fmt.Fprintf(os.Stderr, "%s: target %q not found in PATH\n", testFile, resolvedTarget)
			summary.Failed++
			continue
		}

		var targetExec Target
		if resolvedMode == "process" {
			if sharedProcTarget != nil {
				targetExec = sharedProcTarget
			} else {
				procTarget, err := NewProcessTarget(ctx, resolvedTarget, resolvedArgs...)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating process target for %s: %v\n", testFile, err)
					summary.Failed++
					continue
				}
				targetExec = procTarget
			}
		} else {
			targetExec = NewLocalTarget(resolvedTarget, resolvedArgs, nil, resolvedMode)
		}

		platform := DetectPlatform(ctx, targetExec)

		sections, err := tf.GetSections(opts.RunSections)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting sections from %s: %v\n", testFile, err)
			closeTarget(targetExec)
			summary.Failed++
			continue
		}

		fixture := &FixtureFile{
			Source:           testFile,
			RecordedWith:     resolvedTarget,
			RecordedWithArgs: resolvedArgs,
			Platform:         platform,
			Sections:         make(map[string][]FixtureCommand),
		}

		for _, section := range sections {
			var commands []FixtureCommand
			for _, tc := range section.Commands {
				result, err := targetExec.Exec(ctx, tc.Cmd)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error executing command in %s: %v\n", testFile, err)
					continue
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

		closeTarget(targetExec)

		written, err := WriteFixture(fixturePath, fixture, opts.Indent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing fixture %s: %v\n", fixturePath, err)
			summary.Failed++
			continue
		}

		if written {
			fmt.Fprintf(os.Stderr, "Recorded %s -> %s\n", testFile, fixturePath)
			summary.Recorded++
			summary.FixturePaths = append(summary.FixturePaths, fixturePath)
		} else {
			fmt.Fprintf(os.Stderr, "Unchanged %s\n", testFile)
			summary.Unchanged++
		}
	}

	fmt.Fprintf(os.Stderr, "\nSummary: %d recorded, %d unchanged, %d skipped, %d failed\n",
		summary.Recorded, summary.Unchanged, summary.Skipped, summary.Failed)

	// Refresh the manifest so the baseline on disk stays protected.
	manifest, err := ScanManifest(opts.TestsDir, opts.FixturesDir)
	if err != nil {
		return nil, fmt.Errorf("scanning baseline for manifest: %w", err)
	}
	if err := WriteManifest(opts.FixturesDir, manifest); err != nil {
		return nil, fmt.Errorf("writing manifest: %w", err)
	}

	return summary, nil
}

// ResolveTarget combines CLI flags with a test file's metadata directives:
// the CLI value wins, the file directive is the fallback.
func ResolveTarget(filename string, metadata map[string][]string, cliTarget string, cliTargetArgs []string, cliMode string) (target string, targetArgs []string, mode string, err error) {
	// Resolve target: CLI wins, file directive is fallback
	target = cliTarget
	if target == "" {
		if vals, ok := metadata["target"]; ok {
			if len(vals) > 1 {
				return "", nil, "", fmt.Errorf("%s: multiple # target directives; only one is allowed", filename)
			}
			target = vals[0]
		}
	}
	if target == "" {
		return "", nil, "", fmt.Errorf("%s: no target; add a # target directive or pass --target", filename)
	}

	// Resolve target-arg: CLI wins, file directive is fallback
	targetArgs = cliTargetArgs
	if len(targetArgs) == 0 {
		if vals, ok := metadata["target-arg"]; ok {
			targetArgs = vals
		}
	}

	// Resolve mode: CLI wins, file directive is fallback
	mode = cliMode
	if mode == "" {
		if vals, ok := metadata["mode"]; ok {
			mode = vals[0]
		}
	}
	if mode == "" {
		mode = "shell"
	}
	if mode != "shell" && mode != "process" && mode != "wrap" {
		return "", nil, "", fmt.Errorf("invalid mode %q, must be shell, process, or wrap", mode)
	}

	return target, targetArgs, mode, nil
}

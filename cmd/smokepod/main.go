package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/peteretelej/smokepod/internal/testfile"
	"github.com/peteretelej/smokepod/pkg/smokepod"
	"github.com/urfave/cli/v3"
)

// Exit codes
const (
	exitSuccess      = 0
	exitTestFailure  = 1
	exitConfigError  = 2
	exitRuntimeError = 3
)

func main() {
	cmd := &cli.Command{
		Name:    "smokepod",
		Usage:   "Smoke test runner for CLI and containerized applications",
		Version: smokepod.VersionString(),
		Commands: []*cli.Command{
			runCommand(),
			validateCommand(),
			recordCommand(),
			verifyCommand(),
			updateCommand(),
			probeCommand(),
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		os.Exit(exitRuntimeError)
	}
}

func runCommand() *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "Run tests from config file",
		ArgsUsage: "<config.yaml>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "fail-fast",
				Usage: "Stop on first test failure",
			},
			&cli.BoolFlag{
				Name:  "sequential",
				Usage: "Run tests sequentially (default: parallel)",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "Global timeout (default: from config or 5m)",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Write JSON to file instead of stdout",
			},
			&cli.BoolFlag{
				Name:  "pretty",
				Usage: "Pretty-print JSON output",
			},
		},
		Action: runAction,
	}
}

func validateCommand() *cli.Command {
	return &cli.Command{
		Name:      "validate",
		Usage:     "Validate config without running tests",
		ArgsUsage: "<config.yaml>",
		Action:    validateAction,
	}
}

func recordCommand() *cli.Command {
	return &cli.Command{
		Name:  "record",
		Usage: "Record command outputs as fixtures",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "target",
				Usage: "Path to the target executable",
			},
			&cli.StringSliceFlag{
				Name:  "target-arg",
				Usage: "Fixed argument for the target executable (repeatable)",
			},
			&cli.StringFlag{
				Name:     "tests",
				Usage:    "Path to test files",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "fixtures",
				Usage:    "Output directory for fixtures",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "update",
				Usage: "Overwrite existing fixtures",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "Command timeout",
				Value: 30 * time.Second,
			},
			&cli.StringFlag{
				Name:  "mode",
				Usage: "Target mode: shell (default), process, or wrap",
				Value: "",
			},
			&cli.StringFlag{
				Name:  "run",
				Usage: "Run specific sections (comma-separated)",
			},
			&cli.BoolFlag{
				Name:  "allow-empty",
				Usage: "Allow empty test discovery (no .test files found)",
			},
			&cli.StringFlag{
				Name:  "indent",
				Usage: `JSON indent: "2" (default), "4", or "tab"`,
				Value: "2",
			},
		},
		Action: recordAction,
	}
}

func verifyCommand() *cli.Command {
	return &cli.Command{
		Name:  "verify",
		Usage: "Verify command outputs against fixtures",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "target",
				Usage: "Path to the target executable",
			},
			&cli.StringSliceFlag{
				Name:  "target-arg",
				Usage: "Fixed argument for the target executable (repeatable)",
			},
			&cli.StringFlag{
				Name:     "tests",
				Usage:    "Path to test files",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "fixtures",
				Usage:    "Path to fixtures directory",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "mode",
				Usage: "Target mode: shell (default), process, or wrap",
				Value: "",
			},
			&cli.BoolFlag{
				Name:  "fail-fast",
				Usage: "Stop on first failure",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "Command timeout",
				Value: 30 * time.Second,
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "JSON output",
			},
			&cli.StringFlag{
				Name:  "run",
				Usage: "Run specific sections (comma-separated)",
			},
			&cli.BoolFlag{
				Name:  "allow-empty",
				Usage: "Allow empty test discovery (no .test files found)",
			},
			&cli.IntFlag{
				Name:  "repeat",
				Usage: "Run every section k times; all runs must match to pass (default 1)",
				Value: 1,
			},
		},
		Action: verifyAction,
	}
}

func updateCommand() *cli.Command {
	return &cli.Command{
		Name:  "update",
		Usage: "Review and accept baseline changes by re-recording fixtures",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "target",
				Usage: "Path to the target executable",
			},
			&cli.StringSliceFlag{
				Name:  "target-arg",
				Usage: "Fixed argument for the target executable (repeatable)",
			},
			&cli.StringFlag{
				Name:     "tests",
				Usage:    "Path to test files",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "fixtures",
				Usage:    "Path to fixtures directory",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "mode",
				Usage: "Target mode: shell (default), process, or wrap",
				Value: "",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "Command timeout",
				Value: 30 * time.Second,
			},
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Re-record every fixture, not only drifted ones",
			},
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Apply without interactive confirmation (for scripts and agents)",
			},
			&cli.StringFlag{
				Name:  "indent",
				Usage: `JSON indent for rewritten fixtures: "2" (default), "4", or "tab"`,
				Value: "2",
			},
		},
		Action: updateAction,
	}
}

func probeCommand() *cli.Command {
	return &cli.Command{
		Name:  "probe",
		Usage: "Self-test that verify detects tampered fixtures",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "target",
				Usage: "Path to the target executable",
			},
			&cli.StringSliceFlag{
				Name:  "target-arg",
				Usage: "Fixed argument for the target executable (repeatable)",
			},
			&cli.StringFlag{
				Name:     "tests",
				Usage:    "Path to test files",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "fixtures",
				Usage:    "Path to fixtures directory",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "mode",
				Usage: "Target mode: shell (default), process, or wrap",
				Value: "",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "Command timeout",
				Value: 30 * time.Second,
			},
			&cli.BoolFlag{
				Name:  "target-perturb",
				Usage: "Perturb the live target (append a marker to its stdout) and require verify to detect it",
			},
		},
		Action: probeAction,
	}
}

func runAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 {
		return cli.Exit("Error: config file path required\n  Usage: smokepod run <config.yaml>", exitConfigError)
	}

	configPath := cmd.Args().First()

	// Parse config
	config, err := smokepod.ParseConfig(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cli.Exit(fmt.Sprintf("Config file not found: %s\n  Create a smokepod.yaml file or specify a different path.", configPath), exitConfigError)
		}
		return cli.Exit(fmt.Sprintf("Error: %v", err), exitConfigError)
	}

	// Build options from flags
	var opts []smokepod.Option

	if cmd.Bool("fail-fast") {
		opts = append(opts, smokepod.OptFailFast(true))
	}

	if cmd.Bool("sequential") {
		opts = append(opts, smokepod.OptParallel(false))
	}

	if timeout := cmd.Duration("timeout"); timeout > 0 {
		opts = append(opts, smokepod.OptTimeout(timeout))
	}

	// Set base directory to config file's directory
	opts = append(opts, smokepod.OptBaseDir(configDir(configPath)))

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Run tests
	result, err := smokepod.RunWithOptions(ctx, *config, opts...)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error: %v", err), exitRuntimeError)
	}

	// Setup output
	output := os.Stdout
	if outputPath := cmd.String("output"); outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error creating output file: %v", err), exitRuntimeError)
		}
		defer func() { _ = f.Close() }()
		output = f
	}

	// Report results
	reporter := smokepod.NewReporter(output)
	reporter.SetPretty(cmd.Bool("pretty"))
	if err := reporter.Report(result); err != nil {
		return cli.Exit(fmt.Sprintf("Error writing report: %v", err), exitRuntimeError)
	}

	// Exit code based on test results
	if !result.Passed {
		return cli.Exit("", exitTestFailure)
	}
	return nil
}

func validateAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 {
		return cli.Exit("Error: config file path required\n  Usage: smokepod validate <config.yaml>", exitConfigError)
	}

	configPath := cmd.Args().First()

	config, err := smokepod.ParseConfig(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cli.Exit(fmt.Sprintf("Config file not found: %s\n  Create a smokepod.yaml file or specify a different path.", configPath), exitConfigError)
		}
		return cli.Exit(fmt.Sprintf("Error: %v", err), exitConfigError)
	}

	if err := smokepod.ValidateConfig(config); err != nil {
		return cli.Exit(fmt.Sprintf("Error: %v", err), exitConfigError)
	}

	fmt.Fprintf(os.Stderr, "Config is valid: %s\n", configPath)
	fmt.Fprintf(os.Stderr, "  Name: %s\n", config.Name)
	fmt.Fprintf(os.Stderr, "  Tests: %d\n", len(config.Tests))
	fmt.Fprintf(os.Stderr, "  Timeout: %s\n", config.Settings.Timeout)
	fmt.Fprintf(os.Stderr, "  Parallel: %v\n", config.Settings.IsParallel())
	return nil
}

func recordAction(ctx context.Context, cmd *cli.Command) error {
	cliTarget := cmd.String("target")
	cliTargetArgs := cmd.StringSlice("target-arg")
	testsPath := cmd.String("tests")
	fixturesPath := cmd.String("fixtures")
	update := cmd.Bool("update")
	timeout := cmd.Duration("timeout")
	cliMode := cmd.String("mode")
	runFlag := cmd.String("run")
	allowEmpty := cmd.Bool("allow-empty")

	indent, err := parseIndent(cmd.String("indent"))
	if err != nil {
		return cli.Exit(err.Error(), exitConfigError)
	}

	if os.Getenv("CI") != "" && !update {
		fmt.Fprintln(os.Stderr, "Warning: CI environment detected; use --update to overwrite fixtures")
		return cli.Exit(fmt.Sprintf("Error: %v", smokepod.ErrCIGuard), exitRuntimeError)
	}

	var runSections []string
	if runFlag != "" {
		runSections = strings.Split(runFlag, ",")
		for i, s := range runSections {
			runSections[i] = strings.TrimSpace(s)
		}
	}

	testFiles, err := smokepod.FindTestFiles(testsPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error finding test files: %v", err), exitConfigError)
	}

	if len(testFiles) == 0 {
		if !allowEmpty {
			return cli.Exit("no .test files found in "+testsPath+"; use --allow-empty to allow", exitConfigError)
		}
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
		defer timeoutCancel()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Create shared ProcessTarget when CLI provides both --mode process and --target
	var sharedProcTarget *smokepod.ProcessTarget
	if cliMode == "process" && cliTarget != "" {
		if _, err := exec.LookPath(cliTarget); err != nil {
			return cli.Exit(fmt.Sprintf("target %q not found in PATH", cliTarget), exitConfigError)
		}
		pt, err := smokepod.NewProcessTarget(ctx, cliTarget, cliTargetArgs...)
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error creating shared process target: %v", err), exitRuntimeError)
		}
		sharedProcTarget = pt
		defer func() { _ = sharedProcTarget.Close() }()
	}

	closeTarget := func(t smokepod.Target) {
		if t != nil && t != sharedProcTarget {
			_ = t.Close()
		}
	}

	recorded := 0
	unchanged := 0
	skipped := 0
	failed := 0

	for _, testFile := range testFiles {
		fixturePath := smokepod.FixturePathFromTest(testFile, testsPath, fixturesPath)

		if !update {
			if _, err := os.Stat(fixturePath); err == nil {
				fmt.Fprintf(os.Stderr, "Skipping %s (fixture exists, use --update to overwrite)\n", testFile)
				skipped++
				continue
			}
		}

		tf, err := testfile.Parse(testFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", testFile, err)
			failed++
			continue
		}

		for _, w := range tf.Warnings {
			fmt.Fprintf(os.Stderr, "Warning: %s: %s\n", testFile, w)
		}

		resolvedTarget, resolvedArgs, resolvedMode, err := resolveTarget(testFile, tf.Metadata, cliTarget, cliTargetArgs, cliMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			failed++
			continue
		}

		if _, err := exec.LookPath(resolvedTarget); err != nil {
			fmt.Fprintf(os.Stderr, "%s: target %q not found in PATH\n", testFile, resolvedTarget)
			failed++
			continue
		}

		var targetExec smokepod.Target
		if resolvedMode == "process" {
			if sharedProcTarget != nil {
				targetExec = sharedProcTarget
			} else {
				procTarget, err := smokepod.NewProcessTarget(ctx, resolvedTarget, resolvedArgs...)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating process target for %s: %v\n", testFile, err)
					failed++
					continue
				}
				targetExec = procTarget
			}
		} else {
			targetExec = smokepod.NewLocalTarget(resolvedTarget, resolvedArgs, nil, resolvedMode)
		}

		platform := smokepod.DetectPlatform(ctx, targetExec)

		sections, err := tf.GetSections(runSections)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting sections from %s: %v\n", testFile, err)
			closeTarget(targetExec)
			failed++
			continue
		}

		fixture := &smokepod.FixtureFile{
			Source:           testFile,
			RecordedWith:     resolvedTarget,
			RecordedWithArgs: resolvedArgs,
			Platform:         platform,
			Sections:         make(map[string][]smokepod.FixtureCommand),
		}

		for _, section := range sections {
			var commands []smokepod.FixtureCommand
			for _, tc := range section.Commands {
				result, err := targetExec.Exec(ctx, tc.Cmd)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error executing command in %s: %v\n", testFile, err)
					continue
				}

				commands = append(commands, smokepod.FixtureCommand{
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

		written, err := smokepod.WriteFixture(fixturePath, fixture, indent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing fixture %s: %v\n", fixturePath, err)
			failed++
			continue
		}

		if written {
			fmt.Fprintf(os.Stderr, "Recorded %s -> %s\n", testFile, fixturePath)
			recorded++
		} else {
			fmt.Fprintf(os.Stderr, "Unchanged %s\n", testFile)
			unchanged++
		}
	}

	fmt.Fprintf(os.Stderr, "\nSummary: %d recorded, %d unchanged, %d skipped, %d failed\n", recorded, unchanged, skipped, failed)

	// Refresh the manifest so the baseline on disk stays protected.
	manifest, err := smokepod.ScanManifest(testsPath, fixturesPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error scanning baseline for manifest: %v", err), exitRuntimeError)
	}
	if err := smokepod.WriteManifest(fixturesPath, manifest); err != nil {
		return cli.Exit(fmt.Sprintf("Error writing manifest: %v", err), exitRuntimeError)
	}

	if failed > 0 && recorded == 0 {
		return cli.Exit("all test files failed to record", exitTestFailure)
	}
	return nil
}

func verifyAction(ctx context.Context, cmd *cli.Command) error {
	cliTarget := cmd.String("target")
	cliTargetArgs := cmd.StringSlice("target-arg")
	testsPath := cmd.String("tests")
	fixturesPath := cmd.String("fixtures")
	cliMode := cmd.String("mode")
	failFast := cmd.Bool("fail-fast")
	timeout := cmd.Duration("timeout")
	jsonOutput := cmd.Bool("json")
	allowEmpty := cmd.Bool("allow-empty")
	repeat := cmd.Int("repeat")
	if repeat < 1 {
		return cli.Exit(fmt.Sprintf("--repeat must be >= 1, got %d", repeat), exitConfigError)
	}

	testFiles, err := smokepod.FindTestFiles(testsPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error finding test files: %v", err), exitConfigError)
	}

	if len(testFiles) == 0 {
		if !allowEmpty {
			return cli.Exit("no .test files found in "+testsPath+"; use --allow-empty to allow", exitConfigError)
		}
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
		defer timeoutCancel()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	return runVerify(ctx, cliTarget, cliTargetArgs, cliMode, testFiles, testsPath, fixturesPath, failFast, jsonOutput, cmd.String("run"), repeat)
}

func runVerify(ctx context.Context, cliTarget string, cliTargetArgs []string, cliMode string, testFiles []string, testsPath, fixturesPath string, failFast, jsonOutput bool, runFlag string, repeat int) error {
	// Baseline protection: load the recorded manifest and compare it with the
	// current state on disk before running anything.
	recorded, err := smokepod.LoadManifest(fixturesPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cli.Exit("no "+smokepod.ManifestFileName+" in "+fixturesPath+"; baselines are protected - run `smokepod record` once with the same --tests/--fixtures to create one", exitConfigError)
		}
		return cli.Exit(fmt.Sprintf("Error reading manifest: %v", err), exitRuntimeError)
	}
	onDisk, err := smokepod.ScanManifest(testsPath, fixturesPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error scanning baseline: %v", err), exitRuntimeError)
	}
	drift := smokepod.DiffManifest(recorded, onDisk)

	var runSections []string
	if runFlag != "" {
		runSections = strings.Split(runFlag, ",")
		for i, s := range runSections {
			runSections[i] = strings.TrimSpace(s)
		}
	}

	// Create shared ProcessTarget when CLI provides both --mode process and --target
	var sharedProcTarget *smokepod.ProcessTarget
	if cliMode == "process" && cliTarget != "" {
		if _, err := exec.LookPath(cliTarget); err != nil {
			return cli.Exit(fmt.Sprintf("target %q not found in PATH", cliTarget), exitConfigError)
		}
		pt, err := smokepod.NewProcessTarget(ctx, cliTarget, cliTargetArgs...)
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error creating shared process target: %v", err), exitRuntimeError)
		}
		sharedProcTarget = pt
		defer func() { _ = sharedProcTarget.Close() }()
	}

	closeTarget := func(t smokepod.Target) {
		if t != nil && t != sharedProcTarget {
			_ = t.Close()
		}
	}

	reporter := smokepod.NewVerifyReporter(os.Stderr)

	// Structural failures (stale/missing sections, command count mismatch)
	// take precedence over manifest drift messaging: they already pinpoint
	// the remediation for structure changes.
	structuralFailure := false

	sectionsPassed := 0
	sectionsFailed := 0
	sectionsXFail := 0
	sectionsXPass := 0
	sectionsTotal := 0
	var allSectionResults []smokepod.SectionResult
	var changeEntries []smokepod.ChangeEntry

	for _, testFile := range testFiles {
		fixturePath := smokepod.FixturePathFromTest(testFile, testsPath, fixturesPath)

		fixture, err := smokepod.ReadFixture(fixturePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading fixture %s: %v\n", fixturePath, err)
			sectionsFailed++
			sectionsTotal++
			if failFast {
				break
			}
			continue
		}

		tf, err := testfile.Parse(testFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", testFile, err)
			sectionsFailed++
			sectionsTotal++
			if failFast {
				break
			}
			continue
		}

		for _, w := range tf.Warnings {
			fmt.Fprintf(os.Stderr, "Warning: %s: %s\n", testFile, w)
		}

		resolvedTarget, resolvedArgs, resolvedMode, err := resolveTarget(testFile, tf.Metadata, cliTarget, cliTargetArgs, cliMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			sectionsFailed++
			sectionsTotal++
			if failFast {
				break
			}
			continue
		}

		if _, err := exec.LookPath(resolvedTarget); err != nil {
			fmt.Fprintf(os.Stderr, "%s: target %q not found in PATH\n", testFile, resolvedTarget)
			sectionsFailed++
			sectionsTotal++
			if failFast {
				break
			}
			continue
		}

		var targetExec smokepod.Target
		if resolvedMode == "process" {
			if sharedProcTarget != nil {
				targetExec = sharedProcTarget
			} else {
				procTarget, err := smokepod.NewProcessTarget(ctx, resolvedTarget, resolvedArgs...)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating process target for %s: %v\n", testFile, err)
					sectionsFailed++
					sectionsTotal++
					if failFast {
						break
					}
					continue
				}
				targetExec = procTarget
			}
		} else {
			targetExec = smokepod.NewLocalTarget(resolvedTarget, resolvedArgs, nil, resolvedMode)
		}

		// Get sections to verify from .test file
		// When --run is specified, only get sections that exist in the .test file
		// (missing ones are handled separately below)
		var sections []*testfile.Section
		if len(runSections) > 0 {
			for _, name := range runSections {
				if s := tf.GetSection(name); s != nil {
					sections = append(sections, s)
				}
			}
		} else {
			sections, err = tf.GetSections(nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting sections from %s: %v\n", testFile, err)
				sectionsFailed++
				sectionsTotal++
				closeTarget(targetExec)
				if failFast {
					break
				}
				continue
			}
		}

		// Build set of .test section names for quick lookup
		testSectionSet := make(map[string]bool, len(tf.Sections))
		for name := range tf.Sections {
			testSectionSet[name] = true
		}

		// Build the --run filter set (nil when --run is not specified)
		var runFilterSet map[string]bool
		if len(runSections) > 0 {
			runFilterSet = make(map[string]bool, len(runSections))
			for _, name := range runSections {
				runFilterSet[name] = true
			}
		}

		// Check for stale fixture sections
		staleFailed := false
		for fixtureSectionName := range fixture.Sections {
			if testSectionSet[fixtureSectionName] {
				// Section exists in .test file: not stale
				continue
			}
			// Section is in fixture but not in .test file
			if runFilterSet != nil && !runFilterSet[fixtureSectionName] {
				// --run is specified and this section is outside the filter: ignore
				continue
			}
			// Stale: either --run is not specified (full scope) or section is in --run filter
			reporter.ReportFailure(fixtureSectionName, fmt.Sprintf(
				"stale fixture section: exists in fixture but not in .test file (fixture: %s)", fixturePath,
			))
			reporter.ReportSection(fixtureSectionName, "fail")
			sectionsFailed++
			sectionsTotal++
			allSectionResults = append(allSectionResults, smokepod.SectionResult{Name: fixtureSectionName, Status: "fail"})
			staleFailed = true
			structuralFailure = true
			if failFast {
				break
			}
		}
		if staleFailed && failFast {
			closeTarget(targetExec)
			break
		}

		// Check for --run sections that don't exist in either .test or fixture
		if runFilterSet != nil {
			runMissingFailed := false
			for _, name := range runSections {
				if testSectionSet[name] {
					continue // exists in .test, will be verified below
				}
				if _, inFixture := fixture.Sections[name]; inFixture {
					continue // already reported as stale above
				}
				// Section doesn't exist anywhere
				reporter.ReportFailure(name, "section not found in .test file or fixture")
				reporter.ReportSection(name, "fail")
				sectionsFailed++
				sectionsTotal++
				allSectionResults = append(allSectionResults, smokepod.SectionResult{Name: name, Status: "fail"})
				runMissingFailed = true
				if failFast {
					break
				}
			}
			if runMissingFailed && failFast {
				closeTarget(targetExec)
				break
			}
		}

		for _, section := range sections {
			sectionPassed := true

			fixtureCommands, hasFixture := fixture.Sections[section.Name]
			if !hasFixture {
				reporter.ReportFailure(section.Name, fmt.Sprintf(
					"missing fixture section: exists in .test but not in fixture (fixture: %s)", fixturePath,
				))
				reporter.ReportSection(section.Name, "fail")
				sectionsFailed++
				sectionsTotal++
				structuralFailure = true
				allSectionResults = append(allSectionResults, smokepod.SectionResult{Name: section.Name, Status: "fail"})
				if failFast {
					break
				}
				continue
			}

			// Command-count mismatch detection (bidirectional)
			testCmdCount := len(section.Commands)
			fixtureCmdCount := len(fixtureCommands)
			commandCount := testCmdCount
			countMismatch := false
			if testCmdCount != fixtureCmdCount {
				commandCount = min(testCmdCount, fixtureCmdCount)
				countMismatch = true
			}

			changeAdded := false
			for i := 0; i < commandCount; i++ {
				tcmd := section.Commands[i]
				failedRuns := 0
				var mismatchDiff string
				var entry smokepod.ChangeEntry
				for run := 1; run <= repeat; run++ {
					result, err := targetExec.Exec(ctx, tcmd.Cmd)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
						sectionPassed = false
						failedRuns++
						continue
					}

					expected := fixtureCommands[i]

					testExpected := section.Commands[i].Expected
					stdoutResult := smokepod.CompareOutputWithExpectations(
						expected.Stdout, result.Stdout, testExpected, false, tf.Normalize)
					stderrResult := smokepod.CompareOutputWithExpectations(
						expected.Stderr, result.Stderr, testExpected, true, tf.Normalize)
					exitMatched := smokepod.CompareExitCode(expected.ExitCode, result.ExitCode)

					if !stdoutResult.Matched || !stderrResult.Matched || !exitMatched {
						sectionPassed = false
						failedRuns++

						// Keep the first divergent run's diff and pair so the
						// report stays compact when runs disagree.
						if mismatchDiff == "" {
							var diffParts []string
							if !stdoutResult.Matched {
								diffParts = append(diffParts, "stdout:\n"+stdoutResult.Diff)
							}
							if !stderrResult.Matched {
								diffParts = append(diffParts, "stderr:\n"+stderrResult.Diff)
							}
							if !exitMatched {
								diffParts = append(diffParts, fmt.Sprintf("Exit code: expected %d, got %d", expected.ExitCode, result.ExitCode))
							}
							mismatchDiff = strings.Join(diffParts, "\n")

							entry = smokepod.ChangeEntry{
								File:     testFile,
								Section:  section.Name,
								Line:     stdoutResult.FirstLine,
								Expected: stdoutResult.FirstExpected,
								Actual:   stdoutResult.FirstActual,
							}
							if stdoutResult.Matched {
								entry.Line = stderrResult.FirstLine
								entry.Expected = stderrResult.FirstExpected
								entry.Actual = stderrResult.FirstActual
								if stderrResult.Matched {
									entry.Line = 0
									entry.Expected = fmt.Sprintf("exit %d", expected.ExitCode)
									entry.Actual = fmt.Sprintf("exit %d", result.ExitCode)
								}
							}
						}
					}
				}

				if failedRuns > 0 && mismatchDiff != "" && !section.XFail {
					name := fmt.Sprintf("%s / %s", section.Name, tcmd.Cmd)
					if repeat > 1 {
						name += fmt.Sprintf(" (%d/%d runs failed)", failedRuns, repeat)
					}
					reporter.ReportFailure(name, mismatchDiff)
					if !changeAdded {
						changeAdded = true
						changeEntries = append(changeEntries, entry)
					}
				}
			}

			if countMismatch {
				reporter.ReportFailure(section.Name, fmt.Sprintf(
					"command count mismatch: .test has %d commands, fixture has %d (fixture: %s)",
					testCmdCount, fixtureCmdCount, fixturePath,
				))
				sectionPassed = false
				structuralFailure = true
			}

			// Determine section status
			sectionsTotal++
			var status string
			switch {
			case section.XFail && !sectionPassed:
				status = "xfail"
				sectionsXFail++
			case section.XFail && sectionPassed:
				status = "xpass"
				sectionsXPass++
				reporter.ReportXPass(section.Name, section.XFailReason, testFile, section.Line)
			case !section.XFail && sectionPassed:
				status = "pass"
				sectionsPassed++
			default:
				status = "fail"
				sectionsFailed++
			}

			reporter.ReportSection(section.Name, status)
			allSectionResults = append(allSectionResults, smokepod.SectionResult{
				Name:        section.Name,
				Status:      status,
				XFailReason: section.XFailReason,
			})

			if (status == "fail" || status == "xpass") && failFast {
				break
			}
		}

		closeTarget(targetExec)

		if (sectionsFailed > 0 || sectionsXPass > 0) && failFast {
			break
		}
	}

	reporter.ReportChangeSummary(changeEntries)
	reporter.ReportSummary(sectionsPassed, sectionsFailed, sectionsXFail, sectionsXPass, sectionsTotal)

	if jsonOutput {
		result := &smokepod.Result{
			Name:      "verify",
			Timestamp: time.Now(),
			Passed:    sectionsFailed == 0 && sectionsXPass == 0,
			Summary: smokepod.Summary{
				Total:  sectionsTotal,
				Passed: sectionsPassed,
				Failed: sectionsFailed,
				XFail:  sectionsXFail,
				XPass:  sectionsXPass,
			},
			Tests: []smokepod.TestResult{{
				Name:     "verify",
				Type:     "cli",
				Passed:   sectionsFailed == 0 && sectionsXPass == 0,
				Sections: allSectionResults,
			}},
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error marshaling JSON: %v", err), exitRuntimeError)
		}
		fmt.Println(string(data))
	}

	if len(drift) > 0 && !structuralFailure {
		reporter.ReportManifestDrift(drift)
		return cli.Exit("", exitTestFailure)
	}

	if sectionsFailed > 0 || sectionsXPass > 0 {
		return cli.Exit("", exitTestFailure)
	}
	return nil
}

func resolveTarget(filename string, metadata map[string][]string, cliTarget string, cliTargetArgs []string, cliMode string) (target string, targetArgs []string, mode string, err error) {
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

func configDir(path string) string {
	// Get directory of config file for relative path resolution
	dir := path
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' || dir[i] == '\\' {
			return dir[:i]
		}
	}
	return "."
}

func updateAction(ctx context.Context, cmd *cli.Command) error {
	cliTarget := cmd.String("target")
	cliTargetArgs := cmd.StringSlice("target-arg")
	testsPath := cmd.String("tests")
	fixturesPath := cmd.String("fixtures")
	cliMode := cmd.String("mode")
	timeout := cmd.Duration("timeout")
	all := cmd.Bool("all")
	yes := cmd.Bool("yes")

	indent, err := parseIndent(cmd.String("indent"))
	if err != nil {
		return cli.Exit(err.Error(), exitConfigError)
	}

	// Same environment guard as record, but stricter: update never runs in
	// CI. CI approvals go through reviewed PRs, not flags.
	if os.Getenv("CI") != "" {
		fmt.Fprintln(os.Stderr, "Warning: CI environment detected; baseline updates require human review")
		return cli.Exit("update cannot approve baseline changes in CI; commit reviewed updates through a PR", exitRuntimeError)
	}

	recorded, err := smokepod.LoadManifest(fixturesPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cli.Exit("no "+smokepod.ManifestFileName+" in "+fixturesPath+"; nothing to update - run `smokepod record` once to create a protected baseline", exitConfigError)
		}
		return cli.Exit(fmt.Sprintf("Error reading manifest: %v", err), exitRuntimeError)
	}

	testFiles, err := smokepod.FindTestFiles(testsPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error finding test files: %v", err), exitConfigError)
	}
	if len(testFiles) == 0 {
		return cli.Exit("no .test files found in "+testsPath, exitConfigError)
	}

	onDisk, err := smokepod.ScanManifest(testsPath, fixturesPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error scanning baseline: %v", err), exitRuntimeError)
	}
	drift := smokepod.DiffManifest(recorded, onDisk)

	// Even with an empty re-record scope (e.g. a deleted .test file), drift
	// still needs the manifest refresh below to be accepted.
	scope := updateScope(testFiles, testsPath, drift, all)
	if len(drift) == 0 && !all {
		fmt.Fprintln(os.Stderr, "Baseline in sync; nothing to update")
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
		defer timeoutCancel()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	var sharedProcTarget *smokepod.ProcessTarget
	if cliMode == "process" && cliTarget != "" {
		if _, err := exec.LookPath(cliTarget); err != nil {
			return cli.Exit(fmt.Sprintf("target %q not found in PATH", cliTarget), exitConfigError)
		}
		pt, err := smokepod.NewProcessTarget(ctx, cliTarget, cliTargetArgs...)
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error creating shared process target: %v", err), exitRuntimeError)
		}
		sharedProcTarget = pt
		defer func() { _ = sharedProcTarget.Close() }()
	}

	type reRecord struct {
		fixturePath string
		oldFixture  *smokepod.FixtureFile
		newFixture  *smokepod.FixtureFile
	}
	var plan []reRecord
	failed := 0
	for _, testFile := range scope {
		newFixture, err := reRecordFixture(ctx, testFile, cliTarget, cliTargetArgs, cliMode, sharedProcTarget)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error re-recording %s: %v\n", testFile, err)
			failed++
			continue
		}
		fixturePath := smokepod.FixturePathFromTest(testFile, testsPath, fixturesPath)
		oldFixture, err := smokepod.ReadFixture(fixturePath)
		if err != nil {
			oldFixture = nil
		}
		plan = append(plan, reRecord{fixturePath: fixturePath, oldFixture: oldFixture, newFixture: newFixture})
	}
	if failed > 0 {
		return cli.Exit(fmt.Sprintf("%d test file(s) failed to re-record; nothing was changed", failed), exitTestFailure)
	}

	// Render old-vs-new for review before anything is written.
	changed := 0
	for _, p := range plan {
		_, _ = fmt.Fprintf(os.Stdout, "--- recorded: %s\n+++ re-recorded\n", p.fixturePath)
		diff := renderFixtureDiff(p.oldFixture, p.newFixture)
		if diff == "" {
			_, _ = fmt.Fprintln(os.Stdout, "no output changes")
		} else {
			changed++
			_, _ = fmt.Fprint(os.Stdout, diff)
		}
		_, _ = fmt.Fprintln(os.Stdout)
	}

	if !yes {
		if !isTerminal(os.Stdin) {
			return cli.Exit("refusing to update baselines without approval; re-run with --yes for non-interactive use", exitRuntimeError)
		}
		fmt.Fprintf(os.Stderr, "Apply update to %d fixture(s) and refresh the manifest? [y/N] ", len(plan))
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			return cli.Exit("update aborted; baselines unchanged", exitRuntimeError)
		}
	}

	written := 0
	for _, p := range plan {
		didWrite, err := smokepod.WriteFixture(p.fixturePath, p.newFixture, indent)
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error writing fixture %s: %v", p.fixturePath, err), exitRuntimeError)
		}
		if didWrite {
			written++
		}
	}

	manifest, err := smokepod.ScanManifest(testsPath, fixturesPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error scanning baseline: %v", err), exitRuntimeError)
	}
	if err := smokepod.WriteManifest(fixturesPath, manifest); err != nil {
		return cli.Exit(fmt.Sprintf("Error writing manifest: %v", err), exitRuntimeError)
	}

	fmt.Fprintf(os.Stderr, "Update complete: %d fixture(s) written, manifest refreshed\n", written)
	return nil
}

const probePerturbMarker = "PROBE-PERTURBED-BEHAVIOR"

func probeAction(ctx context.Context, cmd *cli.Command) error {
	cliTarget := cmd.String("target")
	cliTargetArgs := cmd.StringSlice("target-arg")
	testsPath := cmd.String("tests")
	fixturesPath := cmd.String("fixtures")
	cliMode := cmd.String("mode")
	timeout := cmd.Duration("timeout")

	recorded, err := smokepod.LoadManifest(fixturesPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cli.Exit("no "+smokepod.ManifestFileName+" in "+fixturesPath+"; run `smokepod record` first - probe needs a protected baseline", exitConfigError)
		}
		return cli.Exit(fmt.Sprintf("Error reading manifest: %v", err), exitRuntimeError)
	}

	if cmd.Bool("target-perturb") {
		return probeTargetPerturb(ctx, cliTarget, cliTargetArgs, cliMode, testsPath, fixturesPath, timeout)
	}

	// Pick the first recorded command (sorted for determinism) to perturb.
	keys := make([]string, 0, len(recorded.Fixtures))
	for key := range recorded.Fixtures {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var perturbKey, perturbSection, perturbOrig string
	found := false
	for _, key := range keys {
		fixture, err := smokepod.ReadFixture(filepath.Join(fixturesPath, filepath.FromSlash(key)))
		if err != nil {
			continue
		}
		sectionNames := make([]string, 0, len(fixture.Sections))
		for name := range fixture.Sections {
			sectionNames = append(sectionNames, name)
		}
		sort.Strings(sectionNames)
		for _, name := range sectionNames {
			if cmds := fixture.Sections[name]; len(cmds) > 0 {
				perturbKey, perturbSection, perturbOrig = key, name, cmds[0].Stdout
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return cli.Exit("probe found no recorded command output to perturb in "+fixturesPath, exitRuntimeError)
	}

	testFiles, err := smokepod.FindTestFiles(testsPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error finding test files: %v", err), exitConfigError)
	}
	if len(testFiles) == 0 {
		return cli.Exit("no .test files found in "+testsPath, exitConfigError)
	}

	if timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
		defer timeoutCancel()
	}

	// Work entirely on a copy so the real baseline stays untouched.
	tmpRoot, err := os.MkdirTemp("", "smokepod-probe-")
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error creating probe temp dir: %v", err), exitRuntimeError)
	}
	defer func() { _ = os.RemoveAll(tmpRoot) }()

	probeFixtures := filepath.Join(tmpRoot, "fixtures")
	if err := os.CopyFS(probeFixtures, os.DirFS(fixturesPath)); err != nil {
		return cli.Exit(fmt.Sprintf("Error copying fixtures for probe: %v", err), exitRuntimeError)
	}

	probePath := filepath.Join(probeFixtures, filepath.FromSlash(perturbKey))
	perturbed, err := smokepod.ReadFixture(probePath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error reading copied fixture: %v", err), exitRuntimeError)
	}
	perturbed.Sections[perturbSection][0].Stdout = perturbOrig + "TAMPERED BY PROBE"
	if _, err := smokepod.WriteFixture(probePath, perturbed, "  "); err != nil {
		return cli.Exit(fmt.Sprintf("Error writing perturbed fixture: %v", err), exitRuntimeError)
	}

	output, verifyErr := runVerifyCaptured(ctx, cliTarget, cliTargetArgs, cliMode, testFiles, testsPath, probeFixtures)
	if verifyErr == nil {
		return cli.Exit(fmt.Sprintf("protection failed: verify did not detect tampered %s", perturbKey), exitTestFailure)
	}
	if !strings.Contains(output, perturbKey) {
		return cli.Exit(fmt.Sprintf("protection failed: verify did not name the tampered entry %s", perturbKey), exitTestFailure)
	}

	fmt.Fprintf(os.Stderr, "protection verified: verify detected tampered %s\n", perturbKey)
	return nil
}

// runVerifyCaptured runs the verify flow against the given fixtures directory
// with stderr captured, so the probe can inspect the failure output.
func runVerifyCaptured(ctx context.Context, cliTarget string, cliTargetArgs []string, cliMode string, testFiles []string, testsPath, fixturesPath string) (string, error) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stderr = w
	verifyErr := runVerify(ctx, cliTarget, cliTargetArgs, cliMode, testFiles, testsPath, fixturesPath, false, false, "", 1)
	_ = w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	return buf.String(), verifyErr
}

// probeTargetPerturb wraps the resolved target in a shim that appends a
// marker line to its stdout, then requires verify to fail against the
// untouched fixtures. The perturbation is on the actual side, so the
// manifest still matches and the failure can only come from verify
// detecting a real behavior change in the live target.
func probeTargetPerturb(ctx context.Context, cliTarget string, cliTargetArgs []string, cliMode string, testsPath, fixturesPath string, timeout time.Duration) error {
	if timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
		defer timeoutCancel()
	}

	testFiles, err := smokepod.FindTestFiles(testsPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error finding test files: %v", err), exitConfigError)
	}
	if len(testFiles) == 0 {
		return cli.Exit("no .test files found in "+testsPath, exitConfigError)
	}

	// Shim the first test file's target (deterministic walk order) and
	// verify only that file: one detected change is the proof.
	targetFile := testFiles[0]
	tf, err := testfile.Parse(targetFile)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error parsing %s: %v", targetFile, err), exitConfigError)
	}
	resolvedTarget, resolvedArgs, resolvedMode, err := resolveTarget(targetFile, tf.Metadata, cliTarget, cliTargetArgs, cliMode)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error: %v", err), exitConfigError)
	}
	if resolvedMode == "process" {
		return cli.Exit("probe --target-perturb does not support process mode: a shim cannot append to a persistent target's recorded stdout", exitConfigError)
	}

	tmpRoot, err := os.MkdirTemp("", "smokepod-probe-shim-")
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error creating probe temp dir: %v", err), exitRuntimeError)
	}
	defer func() { _ = os.RemoveAll(tmpRoot) }()

	// Named after the real target so verify's shell-target detection treats
	// the shim exactly like the binary it replaces.
	shimPath := filepath.Join(tmpRoot, filepath.Base(resolvedTarget))
	shim := "#!/bin/sh\n" +
		shQuote(resolvedTarget) + ` "$@"` + "\n" +
		"rc=$?\n" +
		"echo " + probePerturbMarker + "\n" +
		"exit $rc\n"
	if err := os.WriteFile(shimPath, []byte(shim), 0755); err != nil {
		return cli.Exit(fmt.Sprintf("Error writing probe shim: %v", err), exitRuntimeError)
	}

	output, verifyErr := runVerifyCaptured(ctx, shimPath, resolvedArgs, resolvedMode, []string{targetFile}, testsPath, fixturesPath)
	if verifyErr == nil {
		return cli.Exit(fmt.Sprintf("protection failed: verify did not detect the perturbed target %s", resolvedTarget), exitTestFailure)
	}
	if !strings.Contains(output, probePerturbMarker) {
		return cli.Exit(fmt.Sprintf("protection failed: verify failure was not caused by the target perturbation for %s", resolvedTarget), exitTestFailure)
	}

	fmt.Fprintf(os.Stderr, "protection verified: verify detected the perturbed target %s (%s)\n", resolvedTarget, targetFile)
	return nil
}

// shQuote single-quotes s for safe use as one shell word.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// updateScope picks the test files to re-record: everything with --all,
// otherwise the files behind drifted manifest entries (a drifted test file
// implicates its fixture and vice versa).
func updateScope(testFiles []string, testsPath string, drift []smokepod.Drift, all bool) []string {
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
		if driftedTests[key] || driftedFixtures[smokepod.FixtureKeyForTest(key)] {
			scope = append(scope, testFile)
		}
	}
	return scope
}

// reRecordFixture runs a test file's commands and builds the fixture that
// record would write, without writing it.
func reRecordFixture(ctx context.Context, testFile, cliTarget string, cliTargetArgs []string, cliMode string, sharedProcTarget *smokepod.ProcessTarget) (*smokepod.FixtureFile, error) {
	tf, err := testfile.Parse(testFile)
	if err != nil {
		return nil, err
	}

	resolvedTarget, resolvedArgs, resolvedMode, err := resolveTarget(testFile, tf.Metadata, cliTarget, cliTargetArgs, cliMode)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath(resolvedTarget); err != nil {
		return nil, fmt.Errorf("target %q not found in PATH", resolvedTarget)
	}

	var targetExec smokepod.Target
	if resolvedMode == "process" {
		if sharedProcTarget != nil {
			targetExec = sharedProcTarget
		} else {
			procTarget, err := smokepod.NewProcessTarget(ctx, resolvedTarget, resolvedArgs...)
			if err != nil {
				return nil, err
			}
			targetExec = procTarget
		}
	} else {
		targetExec = smokepod.NewLocalTarget(resolvedTarget, resolvedArgs, nil, resolvedMode)
	}
	defer func() {
		if targetExec != nil && targetExec != sharedProcTarget {
			_ = targetExec.Close()
		}
	}()

	platform := smokepod.DetectPlatform(ctx, targetExec)

	sections, err := tf.GetSections(nil)
	if err != nil {
		return nil, err
	}

	fixture := &smokepod.FixtureFile{
		Source:           testFile,
		RecordedWith:     resolvedTarget,
		RecordedWithArgs: resolvedArgs,
		Platform:         platform,
		Sections:         make(map[string][]smokepod.FixtureCommand),
	}
	for _, section := range sections {
		commands := []smokepod.FixtureCommand{}
		for _, tc := range section.Commands {
			result, err := targetExec.Exec(ctx, tc.Cmd)
			if err != nil {
				return nil, fmt.Errorf("executing %q: %w", tc.Cmd, err)
			}
			commands = append(commands, smokepod.FixtureCommand{
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
func renderFixtureDiff(oldFixture, newFixture *smokepod.FixtureFile) string {
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
			var oc, nc *smokepod.FixtureCommand
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

func writeCommandDiff(b *strings.Builder, oc, nc *smokepod.FixtureCommand) {
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

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func parseIndent(v string) (string, error) {
	switch v {
	case "2":
		return "  ", nil
	case "4":
		return "    ", nil
	case "tab":
		return "\t", nil
	default:
		return "", fmt.Errorf("invalid --indent value %q: use 2, 4, or tab", v)
	}
}

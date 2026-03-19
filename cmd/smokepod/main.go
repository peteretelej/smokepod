package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/peteretelej/smokepod/internal/testfile"
	"github.com/peteretelej/smokepod/pkg/smokepod"
	"github.com/urfave/cli/v2"
)

// Exit codes
const (
	exitSuccess      = 0
	exitTestFailure  = 1
	exitConfigError  = 2
	exitRuntimeError = 3
)

func main() {
	app := &cli.App{
		Name:    "smokepod",
		Usage:   "Smoke test runner for CLI and containerized applications",
		Version: smokepod.VersionString(),
		Commands: []*cli.Command{
			runCommand(),
			validateCommand(),
			recordCommand(),
			verifyCommand(),
		},
	}

	if err := app.Run(os.Args); err != nil {
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
		},
		Action: verifyAction,
	}
}

func runAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return cli.Exit("Error: config file path required\n  Usage: smokepod run <config.yaml>", exitConfigError)
	}

	configPath := c.Args().First()

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

	if c.Bool("fail-fast") {
		opts = append(opts, smokepod.OptFailFast(true))
	}

	if c.Bool("sequential") {
		opts = append(opts, smokepod.OptParallel(false))
	}

	if timeout := c.Duration("timeout"); timeout > 0 {
		opts = append(opts, smokepod.OptTimeout(timeout))
	}

	// Set base directory to config file's directory
	opts = append(opts, smokepod.OptBaseDir(configDir(configPath)))

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
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
	if outputPath := c.String("output"); outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error creating output file: %v", err), exitRuntimeError)
		}
		defer func() { _ = f.Close() }()
		output = f
	}

	// Report results
	reporter := smokepod.NewReporter(output)
	reporter.SetPretty(c.Bool("pretty"))
	if err := reporter.Report(result); err != nil {
		return cli.Exit(fmt.Sprintf("Error writing report: %v", err), exitRuntimeError)
	}

	// Exit code based on test results
	if !result.Passed {
		return cli.Exit("", exitTestFailure)
	}
	return nil
}

func validateAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return cli.Exit("Error: config file path required\n  Usage: smokepod validate <config.yaml>", exitConfigError)
	}

	configPath := c.Args().First()

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

func recordAction(c *cli.Context) error {
	cliTarget := c.String("target")
	cliTargetArgs := c.StringSlice("target-arg")
	testsPath := c.String("tests")
	fixturesPath := c.String("fixtures")
	update := c.Bool("update")
	timeout := c.Duration("timeout")
	cliMode := c.String("mode")
	runFlag := c.String("run")
	allowEmpty := c.Bool("allow-empty")

	var indent string
	switch v := c.String("indent"); v {
	case "2":
		indent = "  "
	case "4":
		indent = "    "
	case "tab":
		indent = "\t"
	default:
		return cli.Exit(fmt.Sprintf("invalid --indent value %q: use 2, 4, or tab", v), exitConfigError)
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

	ctx, cancel := context.WithCancel(context.Background())
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
			for _, cmd := range section.Commands {
				result, err := targetExec.Exec(ctx, cmd.Cmd)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error executing command in %s: %v\n", testFile, err)
					continue
				}

				commands = append(commands, smokepod.FixtureCommand{
					Line:     cmd.Line,
					Command:  cmd.Cmd,
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
	if failed > 0 && recorded == 0 {
		return cli.Exit("all test files failed to record", exitTestFailure)
	}
	return nil
}

func verifyAction(c *cli.Context) error {
	cliTarget := c.String("target")
	cliTargetArgs := c.StringSlice("target-arg")
	testsPath := c.String("tests")
	fixturesPath := c.String("fixtures")
	cliMode := c.String("mode")
	failFast := c.Bool("fail-fast")
	timeout := c.Duration("timeout")
	jsonOutput := c.Bool("json")
	allowEmpty := c.Bool("allow-empty")

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

	ctx, cancel := context.WithCancel(context.Background())
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

	return runVerify(c, ctx, cliTarget, cliTargetArgs, cliMode, testFiles, testsPath, fixturesPath, failFast, jsonOutput)
}

func runVerify(c *cli.Context, ctx context.Context, cliTarget string, cliTargetArgs []string, cliMode string, testFiles []string, testsPath, fixturesPath string, failFast bool, jsonOutput bool) error {
	runFlag := c.String("run")

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

	sectionsPassed := 0
	sectionsFailed := 0
	sectionsXFail := 0
	sectionsXPass := 0
	sectionsTotal := 0
	var allSectionResults []smokepod.SectionResult

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

			for i := 0; i < commandCount; i++ {
				cmd := section.Commands[i]
				result, err := targetExec.Exec(ctx, cmd.Cmd)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
					sectionPassed = false
					continue
				}

				expected := fixtureCommands[i]

				testExpected := section.Commands[i].Expected
				stdoutResult := smokepod.CompareOutputWithExpectations(
					expected.Stdout, result.Stdout, testExpected, false)
				stderrResult := smokepod.CompareOutputWithExpectations(
					expected.Stderr, result.Stderr, testExpected, true)
				exitMatched := smokepod.CompareExitCode(expected.ExitCode, result.ExitCode)

				if !stdoutResult.Matched || !stderrResult.Matched || !exitMatched {
					sectionPassed = false

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

					if !section.XFail {
						reporter.ReportFailure(fmt.Sprintf("%s / %s", section.Name, cmd.Cmd), strings.Join(diffParts, "\n"))
					}
				}
			}

			if countMismatch {
				reporter.ReportFailure(section.Name, fmt.Sprintf(
					"command count mismatch: .test has %d commands, fixture has %d (fixture: %s)",
					testCmdCount, fixtureCmdCount, fixturePath,
				))
				sectionPassed = false
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

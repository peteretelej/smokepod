package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
				Name:     "target",
				Usage:    "Path to the target executable",
				Required: true,
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
				Name:  "run",
				Usage: "Run specific sections (comma-separated)",
			},
			&cli.BoolFlag{
				Name:  "allow-empty",
				Usage: "Allow empty test discovery (no .test files found)",
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
				Name:     "target",
				Usage:    "Path to the target executable",
				Required: true,
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
				Usage: "Target mode: shell (default) or process",
				Value: "shell",
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
	target := c.String("target")
	targetArgs := c.StringSlice("target-arg")
	testsPath := c.String("tests")
	fixturesPath := c.String("fixtures")
	update := c.Bool("update")
	timeout := c.Duration("timeout")
	runFlag := c.String("run")
	allowEmpty := c.Bool("allow-empty")

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

	targetExec := smokepod.NewLocalTarget(target, targetArgs, nil)
	platform := smokepod.DetectPlatform(ctx, targetExec)

	recorded := 0
	skipped := 0

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
			continue
		}

		sections, err := tf.GetSections(runSections)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting sections from %s: %v\n", testFile, err)
			continue
		}

		fixture := &smokepod.FixtureFile{
			Source:       testFile,
			RecordedWith: target,
			RecordedAt:   time.Now(),
			Platform:     platform,
			Sections:     make(map[string][]smokepod.FixtureCommand),
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

		if err := smokepod.WriteFixture(fixturePath, fixture); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing fixture %s: %v\n", fixturePath, err)
			continue
		}

		fmt.Fprintf(os.Stderr, "Recorded %s -> %s\n", testFile, fixturePath)
		recorded++
	}

	fmt.Fprintf(os.Stderr, "\nSummary: %d recorded, %d skipped\n", recorded, skipped)
	return nil
}

func verifyAction(c *cli.Context) error {
	target := c.String("target")
	targetArgs := c.StringSlice("target-arg")
	testsPath := c.String("tests")
	fixturesPath := c.String("fixtures")
	mode := c.String("mode")
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

	var targetExec smokepod.Target
	if mode == "process" {
		procTarget, err := smokepod.NewProcessTarget(ctx, target, targetArgs...)
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error creating process target: %v", err), exitRuntimeError)
		}
		defer func() { _ = procTarget.Close() }()
		targetExec = procTarget
	} else {
		targetExec = smokepod.NewLocalTarget(target, targetArgs, nil)
	}

	return runVerify(c, ctx, targetExec, testFiles, testsPath, fixturesPath, failFast, jsonOutput)
}

func runVerify(c *cli.Context, ctx context.Context, targetExec smokepod.Target, testFiles []string, testsPath, fixturesPath string, failFast bool, jsonOutput bool) error {
	runFlag := c.String("run")

	var runSections []string
	if runFlag != "" {
		runSections = strings.Split(runFlag, ",")
		for i, s := range runSections {
			runSections[i] = strings.TrimSpace(s)
		}
	}

	reporter := smokepod.NewVerifyReporter(os.Stderr)

	totalPassed := 0
	totalFailed := 0
	totalCommands := 0

	for _, testFile := range testFiles {
		fixturePath := smokepod.FixturePathFromTest(testFile, testsPath, fixturesPath)

		fixture, err := smokepod.ReadFixture(fixturePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading fixture %s: %v\n", fixturePath, err)
			totalFailed++
			if failFast {
				break
			}
			continue
		}

		tf, err := testfile.Parse(testFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", testFile, err)
			totalFailed++
			if failFast {
				break
			}
			continue
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
				totalFailed++
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
			reporter.ReportSection(fixtureSectionName, false)
			totalFailed++
			staleFailed = true
			if failFast {
				break
			}
		}
		if staleFailed && failFast {
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
				reporter.ReportSection(name, false)
				totalFailed++
				runMissingFailed = true
				if failFast {
					break
				}
			}
			if runMissingFailed && failFast {
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
				reporter.ReportSection(section.Name, false)
				totalFailed++
				if failFast {
					break
				}
				continue
			}

			// Command-count mismatch detection (bidirectional)
			testCmdCount := len(section.Commands)
			fixtureCmdCount := len(fixtureCommands)
			if testCmdCount != fixtureCmdCount {
				reporter.ReportFailure(section.Name, fmt.Sprintf(
					"command count mismatch: .test has %d commands, fixture has %d (fixture: %s)",
					testCmdCount, fixtureCmdCount, fixturePath,
				))
				reporter.ReportSection(section.Name, false)
				totalFailed++
				if failFast {
					break
				}
				continue
			}

			for i, cmd := range section.Commands {
				totalCommands++

				result, err := targetExec.Exec(ctx, cmd.Cmd)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
					totalFailed++
					sectionPassed = false
					if failFast {
						break
					}
					continue
				}

				expected := fixtureCommands[i]

				stdoutResult := smokepod.CompareOutput(expected.Stdout, result.Stdout)
				stderrResult := smokepod.CompareOutput(expected.Stderr, result.Stderr)
				exitMatched := smokepod.CompareExitCode(expected.ExitCode, result.ExitCode)

				if stdoutResult.Matched && stderrResult.Matched && exitMatched {
					totalPassed++
				} else {
					sectionPassed = false
					totalFailed++

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

					reporter.ReportFailure(fmt.Sprintf("%s / %s", section.Name, cmd.Cmd), strings.Join(diffParts, "\n"))
				}
			}

			reporter.ReportSection(section.Name, sectionPassed)

			if !sectionPassed && failFast {
				break
			}
		}

		if totalFailed > 0 && failFast {
			break
		}
	}

	reporter.ReportSummary(totalPassed, totalFailed, totalCommands)

	if jsonOutput {
		result := &smokepod.Result{
			Name:      "verify",
			Timestamp: time.Now(),
			Passed:    totalFailed == 0,
			Summary: smokepod.Summary{
				Total:  totalCommands,
				Passed: totalPassed,
				Failed: totalFailed,
			},
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error marshaling JSON: %v", err), exitRuntimeError)
		}
		fmt.Println(string(data))
	}

	if totalFailed > 0 {
		return cli.Exit("", exitTestFailure)
	}
	return nil
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

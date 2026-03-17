package main

import (
	"context"
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
		Usage:   "Containerized smoke test runner",
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
				Usage:    "Shell to use for recording",
				Required: true,
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
				Usage:    "Target command (shell or process)",
				Required: true,
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
	testsPath := c.String("tests")
	fixturesPath := c.String("fixtures")
	update := c.Bool("update")
	_ = c.Duration("timeout")
	runFlag := c.String("run")

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
		fmt.Fprintf(os.Stderr, "No .test files found in %s\n", testsPath)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	targetExec := smokepod.NewLocalTarget(target, nil)
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
	testsPath := c.String("tests")
	fixturesPath := c.String("fixtures")
	mode := c.String("mode")
	failFast := c.Bool("fail-fast")
	_ = c.Duration("timeout")
	_ = c.Bool("json")

	testFiles, err := smokepod.FindTestFiles(testsPath)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error finding test files: %v", err), exitConfigError)
	}

	if len(testFiles) == 0 {
		fmt.Fprintf(os.Stderr, "No .test files found in %s\n", testsPath)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	var targetExec smokepod.Target
	if mode == "process" {
		procTarget, err := smokepod.NewProcessTarget(ctx, target)
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error creating process target: %v", err), exitRuntimeError)
		}
		defer func() { _ = procTarget.Close() }()
		targetExec = procTarget
	} else {
		targetExec = smokepod.NewLocalTarget(target, nil)
	}

	return runVerify(c, ctx, targetExec, testFiles, testsPath, fixturesPath, failFast)
}

func runVerify(c *cli.Context, ctx context.Context, targetExec smokepod.Target, testFiles []string, testsPath, fixturesPath string, failFast bool) error {
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

		sections, err := tf.GetSections(runSections)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting sections from %s: %v\n", testFile, err)
			totalFailed++
			if failFast {
				break
			}
			continue
		}

		for _, section := range sections {
			sectionPassed := true

			fixtureCommands, hasFixture := fixture.Sections[section.Name]
			if !hasFixture {
				fmt.Fprintf(os.Stderr, "\nMissing fixture for section: %s\n", section.Name)
				reporter.ReportSection(section.Name, false)
				totalFailed++
				sectionPassed = false
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

				if i >= len(fixtureCommands) {
					fmt.Fprintf(os.Stderr, "\nMissing fixture for command: %s\n", cmd.Cmd)
					reporter.ReportSection(section.Name, false)
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

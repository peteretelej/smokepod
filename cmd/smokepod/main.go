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
			initCommand(),
			runCommand(),
			validateCommand(),
			recordCommand(),
			verifyCommand(),
			updateCommand(),
			probeCommand(),
			mcpCommand(),
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

func initCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Scaffold a smoke test and record its baseline",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "target",
				Usage: "Target executable to probe and record (omit for a commented template)",
			},
			&cli.StringFlag{
				Name:  "dir",
				Usage: "Directory for the scaffolded .test file",
				Value: "tests",
			},
			&cli.StringSliceFlag{
				Name:  "probe",
				Usage: `Extra target command to probe (repeatable), e.g. --probe "greet"`,
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Overwrite the .test file and baseline init generates",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Print scaffolded files and next commands as JSON",
			},
		},
		Action: initAction,
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
			&cli.BoolFlag{
				Name:  "legacy-framing",
				Usage: "Process mode: speak the old JSONL protocol (removed in the next release)",
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

func mcpCommand() *cli.Command {
	return &cli.Command{
		Name:   "mcp",
		Usage:  "Run the smokepod MCP server (tools: record, check, update) over stdio",
		Action: mcpAction,
	}
}

func mcpAction(ctx context.Context, cmd *cli.Command) error {
	return smokepod.ServeMCP(ctx, smokepod.VersionString())
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
	indent, err := parseIndent(cmd.String("indent"))
	if err != nil {
		return cli.Exit(err.Error(), exitConfigError)
	}

	if os.Getenv("CI") != "" && !cmd.Bool("update") {
		fmt.Fprintln(os.Stderr, "Warning: CI environment detected; use --update to overwrite fixtures")
		return cli.Exit(fmt.Sprintf("Error: %v", smokepod.ErrCIGuard), exitRuntimeError)
	}

	var runSections []string
	if runFlag := cmd.String("run"); runFlag != "" {
		runSections = strings.Split(runFlag, ",")
		for i, s := range runSections {
			runSections[i] = strings.TrimSpace(s)
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if timeout := cmd.Duration("timeout"); timeout > 0 {
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

	summary, err := smokepod.Record(ctx, smokepod.RecordOptions{
		Target:      cmd.String("target"),
		TargetArgs:  cmd.StringSlice("target-arg"),
		Mode:        cmd.String("mode"),
		TestsDir:    cmd.String("tests"),
		FixturesDir: cmd.String("fixtures"),
		Update:      cmd.Bool("update"),
		RunSections: runSections,
		AllowEmpty:  cmd.Bool("allow-empty"),
		Indent:      indent,
	})
	if err != nil {
		exitCode := exitRuntimeError
		if errors.Is(err, smokepod.ErrNoTestFiles) || errors.Is(err, smokepod.ErrDiscovery) || errors.Is(err, smokepod.ErrTargetNotFound) {
			exitCode = exitConfigError
		}
		return cli.Exit(fmt.Sprintf("Error: %v", err), exitCode)
	}

	if summary.Failed > 0 && summary.Recorded == 0 {
		return cli.Exit("all test files failed to record", exitTestFailure)
	}
	return nil
}

func initAction(ctx context.Context, cmd *cli.Command) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	res, err := smokepod.Init(ctx, smokepod.InitOptions{
		Target: cmd.String("target"),
		Dir:    cmd.String("dir"),
		Probes: cmd.StringSlice("probe"),
		Force:  cmd.Bool("force"),
	})
	if err != nil {
		exitCode := exitRuntimeError
		if errors.Is(err, smokepod.ErrScaffoldExists) || errors.Is(err, smokepod.ErrTargetNotFound) {
			exitCode = exitConfigError
		}
		return cli.Exit(fmt.Sprintf("Error: %v", err), exitCode)
	}

	if cmd.Bool("json") {
		data, err := json.MarshalIndent(struct {
			Files []string `json:"files"`
			Next  []string `json:"next"`
		}{Files: res.Files, Next: res.Next}, "", "  ")
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error marshaling JSON: %v", err), exitRuntimeError)
		}
		fmt.Println(string(data))
		return nil
	}

	for _, f := range res.Files {
		fmt.Fprintf(os.Stderr, "Wrote %s\n", f)
	}
	if len(res.Skipped) > 0 {
		fmt.Fprintf(os.Stderr, "Skipped probes (command failed, commented out): %s\n", strings.Join(res.Skipped, ", "))
	}
	if len(res.Next) > 0 {
		fmt.Println("Next steps:")
		for _, next := range res.Next {
			fmt.Printf("  %s\n", next)
		}
	}
	return nil
}

func verifyAction(ctx context.Context, cmd *cli.Command) error {
	repeat := cmd.Int("repeat")
	if repeat < 1 {
		return cli.Exit(fmt.Sprintf("--repeat must be >= 1, got %d", repeat), exitConfigError)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if timeout := cmd.Duration("timeout"); timeout > 0 {
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

	var runSections []string
	if runFlag := cmd.String("run"); runFlag != "" {
		runSections = strings.Split(runFlag, ",")
		for i, s := range runSections {
			runSections[i] = strings.TrimSpace(s)
		}
	}

	result, err := smokepod.Verify(ctx, smokepod.VerifyOptions{
		Target:        cmd.String("target"),
		TargetArgs:    cmd.StringSlice("target-arg"),
		Mode:          cmd.String("mode"),
		TestsDir:      cmd.String("tests"),
		FixturesDir:   cmd.String("fixtures"),
		RunSections:   runSections,
		FailFast:      cmd.Bool("fail-fast"),
		AllowEmpty:    cmd.Bool("allow-empty"),
		Repeat:        repeat,
		LegacyFraming: cmd.Bool("legacy-framing"),
	})

	if cmd.Bool("json") && result != nil {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return cli.Exit(fmt.Sprintf("Error marshaling JSON: %v", err), exitRuntimeError)
		}
		fmt.Println(string(data))
	}

	if err != nil {
		var usageErr *smokepod.UsageError
		switch {
		case errors.As(err, &usageErr):
			return cli.Exit(err.Error(), exitConfigError)
		case errors.Is(err, smokepod.ErrVerifyFailed):
			return cli.Exit("", exitTestFailure)
		default:
			return cli.Exit(err.Error(), exitRuntimeError)
		}
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

func updateAction(ctx context.Context, cmd *cli.Command) error {
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if timeout := cmd.Duration("timeout"); timeout > 0 {
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

	plan, err := smokepod.PlanUpdate(ctx, smokepod.UpdateOptions{
		Target:      cmd.String("target"),
		TargetArgs:  cmd.StringSlice("target-arg"),
		Mode:        cmd.String("mode"),
		TestsDir:    cmd.String("tests"),
		FixturesDir: cmd.String("fixtures"),
		All:         cmd.Bool("all"),
		Indent:      indent,
	})
	if err != nil {
		var usageErr *smokepod.UsageError
		var reRecordErr *smokepod.ReRecordError
		switch {
		case errors.As(err, &usageErr):
			return cli.Exit(err.Error(), exitConfigError)
		case errors.As(err, &reRecordErr):
			return cli.Exit(err.Error(), exitTestFailure)
		default:
			return cli.Exit(err.Error(), exitRuntimeError)
		}
	}

	if plan.Synced {
		fmt.Fprintln(os.Stderr, "Baseline in sync; nothing to update")
		return nil
	}

	// Render old-vs-new for review before anything is written.
	_, _ = fmt.Fprint(os.Stdout, plan.RenderDiff())

	if !cmd.Bool("yes") {
		if !isTerminal(os.Stdin) {
			return cli.Exit("refusing to update baselines without approval; re-run with --yes for non-interactive use", exitRuntimeError)
		}
		fmt.Fprintf(os.Stderr, "Apply update to %d fixture(s) and refresh the manifest? [y/N] ", len(plan.Updates))
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			return cli.Exit("update aborted; baselines unchanged", exitRuntimeError)
		}
	}

	written, err := plan.Apply()
	if err != nil {
		return cli.Exit(err.Error(), exitRuntimeError)
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
// with the progress report captured, so the probe can inspect the failure
// output.
func runVerifyCaptured(ctx context.Context, cliTarget string, cliTargetArgs []string, cliMode string, testFiles []string, testsPath, fixturesPath string) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}

	_, verifyErr := smokepod.Verify(ctx, smokepod.VerifyOptions{
		Target:       cliTarget,
		TargetArgs:   cliTargetArgs,
		Mode:         cliMode,
		TestsDir:     testsPath,
		TestFiles:    testFiles,
		FixturesDir:  fixturesPath,
		ReportWriter: w,
	})
	_ = w.Close()

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
	resolvedTarget, resolvedArgs, resolvedMode, err := smokepod.ResolveTarget(targetFile, tf.Metadata, cliTarget, cliTargetArgs, cliMode)
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

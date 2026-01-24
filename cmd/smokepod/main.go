package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/peteretelej/smokepod/pkg/smokepod"
	"github.com/urfave/cli/v2"
)

// Exit codes
const (
	exitSuccess       = 0
	exitTestFailure   = 1
	exitConfigError   = 2
	exitRuntimeError  = 3
)

func main() {
	app := &cli.App{
		Name:    "smokepod",
		Usage:   "Containerized smoke test runner",
		Version: smokepod.VersionString(),
		Commands: []*cli.Command{
			runCommand(),
			validateCommand(),
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

package smokepod

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/peteretelej/smokepod/internal/testfile"
	"github.com/peteretelej/smokepod/pkg/smokepod/runners"
)

func convertSectionResult(r *runners.SectionResult) SectionResult {
	commands := make([]CommandResult, len(r.Commands))
	for i, cmd := range r.Commands {
		commands[i] = CommandResult{
			Command:        cmd.Command,
			Line:           cmd.Line,
			Expected:       cmd.Expected,
			Actual:         cmd.Actual,
			Passed:         cmd.Passed,
			Error:          cmd.Error,
			WhitespaceDiff: cmd.WhitespaceDiff,
		}
	}
	status := "fail"
	if r.Passed {
		status = "pass"
	}
	return SectionResult{
		Name:     r.Name,
		Status:   status,
		Commands: commands,
	}
}

// Executor orchestrates test execution.
type Executor struct {
	config   *Config
	parallel bool
	failFast bool
	timeout  time.Duration
	baseDir  string // base directory for resolving relative paths
}

// ExecutorOption configures the executor.
type ExecutorOption func(*Executor)

// NewExecutor creates a new executor with the given config.
func NewExecutor(cfg *Config, opts ...ExecutorOption) *Executor {
	e := &Executor{
		config:   cfg,
		parallel: cfg.Settings.IsParallel(),
		failFast: cfg.Settings.FailFast,
		timeout:  cfg.Settings.Timeout,
		baseDir:  ".",
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// WithParallel sets whether to run tests in parallel.
func WithParallel(p bool) ExecutorOption {
	return func(e *Executor) {
		e.parallel = p
	}
}

// WithFailFast sets whether to stop on first failure.
func WithFailFast(ff bool) ExecutorOption {
	return func(e *Executor) {
		e.failFast = ff
	}
}

// WithTimeout sets the global timeout for all tests.
func WithTimeout(t time.Duration) ExecutorOption {
	return func(e *Executor) {
		e.timeout = t
	}
}

// WithBaseDir sets the base directory for resolving relative paths.
func WithBaseDir(dir string) ExecutorOption {
	return func(e *Executor) {
		e.baseDir = dir
	}
}

// Execute runs all tests and returns aggregated results.
func (e *Executor) Execute(ctx context.Context) (*Result, error) {
	start := time.Now()

	// Apply global timeout if set
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}

	var results []TestResult
	if e.parallel {
		results = e.executeParallel(ctx)
	} else {
		results = e.executeSequential(ctx)
	}

	return e.aggregate(results, time.Since(start)), nil
}

func (e *Executor) executeSequential(ctx context.Context) []TestResult {
	results := make([]TestResult, 0, len(e.config.Tests))

	for _, test := range e.config.Tests {
		// Check if context is already cancelled
		if ctx.Err() != nil {
			results = append(results, TestResult{
				Name:   test.Name,
				Type:   test.Type,
				Passed: false,
				Error:  "cancelled",
			})
			continue
		}

		result := e.runTest(ctx, test)
		results = append(results, result)

		if e.failFast && !result.Passed {
			// Mark remaining tests as skipped
			for _, remaining := range e.config.Tests[len(results):] {
				results = append(results, TestResult{
					Name:   remaining.Name,
					Type:   remaining.Type,
					Passed: false,
					Error:  "skipped (fail-fast)",
				})
			}
			break
		}
	}

	return results
}

func (e *Executor) executeParallel(ctx context.Context) []TestResult {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstFailure bool

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Use a slice to preserve test order in results
	results := make([]TestResult, len(e.config.Tests))

	for i, test := range e.config.Tests {
		wg.Add(1)
		go func(idx int, t TestDefinition) {
			defer wg.Done()

			result := e.runTest(ctx, t)

			mu.Lock()
			results[idx] = result
			if e.failFast && !result.Passed && !firstFailure {
				firstFailure = true
				cancel() // Cancel remaining tests
			}
			mu.Unlock()
		}(i, test)
	}

	wg.Wait()
	return results
}

func (e *Executor) runTest(ctx context.Context, test TestDefinition) TestResult {
	start := time.Now()

	result := TestResult{
		Name: test.Name,
		Type: test.Type,
	}

	// Check if already cancelled
	if ctx.Err() != nil {
		result.Error = "cancelled"
		result.Duration = time.Since(start)
		return result
	}

	switch test.Type {
	case "cli":
		e.runCLITest(ctx, test, &result)
	case "playwright":
		e.runPlaywrightTest(ctx, test, &result)
	default:
		result.Error = fmt.Sprintf("unknown test type: %s", test.Type)
	}

	result.Duration = time.Since(start)
	return result
}

func (e *Executor) runCLITest(ctx context.Context, test TestDefinition, result *TestResult) {
	testPath := e.resolvePath(test.File)
	tf, err := testfile.Parse(testPath)
	if err != nil {
		result.Error = fmt.Sprintf("parsing test file: %v", err)
		return
	}

	sections, err := tf.GetSections(test.Run)
	if err != nil {
		result.Error = err.Error()
		return
	}

	target, err := e.createTarget(ctx, test)
	if err != nil {
		result.Error = err.Error()
		return
	}
	defer func() { _ = target.Close() }()

	runner := runners.NewCLIRunner(target)
	result.Passed = true

	for _, section := range sections {
		sectionResult, err := runner.Run(ctx, section)
		if err != nil {
			result.Error = fmt.Sprintf("running section %s: %v", section.Name, err)
			result.Passed = false
			return
		}

		result.Sections = append(result.Sections, convertSectionResult(sectionResult))
		if !sectionResult.Passed {
			result.Passed = false
		}
	}
}

func (e *Executor) runPlaywrightTest(ctx context.Context, test TestDefinition, result *TestResult) {
	projectPath := e.resolvePath(test.Path)
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		result.Error = fmt.Sprintf("resolving path: %v", err)
		return
	}

	container, err := NewContainer(ctx, ContainerConfig{
		Image: test.Image,
		Mounts: []Mount{
			{Source: absPath, Target: "/app"},
		},
		Env: map[string]string{
			"CI": "true",
		},
	})
	if err != nil {
		result.Error = fmt.Sprintf("creating container: %v", err)
		return
	}
	defer func() { _ = container.Terminate(context.Background()) }()

	if _, err := container.Exec(ctx, []string{"sh", "-c", "cd /app && npm ci"}); err != nil {
		result.Error = fmt.Sprintf("installing dependencies: %v", err)
		return
	}

	target := NewDockerTarget(container)
	runner := runners.NewPlaywrightRunner(target)
	pwResult, err := runner.Run(ctx, test.Args)
	if err != nil {
		result.Error = fmt.Sprintf("running playwright: %v", err)
		if pwResult != nil {
			result.Passed = pwResult.Passed
		}
		return
	}

	result.Passed = pwResult.Passed
}

func (e *Executor) createTarget(ctx context.Context, test TestDefinition) (Target, error) {
	if test.Image != "" {
		container, err := NewContainer(ctx, ContainerConfig{
			Image: test.Image,
		})
		if err != nil {
			return nil, fmt.Errorf("creating container: %w", err)
		}
		return NewDockerTarget(container), nil
	}

	if test.Target != "" {
		if test.Mode == "process" {
			return NewProcessTarget(ctx, test.Target, test.TargetArgs...)
		}
		return NewLocalTarget(test.Target, test.TargetArgs, nil), nil
	}

	return nil, fmt.Errorf("test must specify image or target")
}

func (e *Executor) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(e.baseDir, path)
}

func (e *Executor) aggregate(results []TestResult, duration time.Duration) *Result {
	summary := Summary{Total: len(results)}
	passed := true

	for _, r := range results {
		switch {
		case r.Passed:
			summary.Passed++
		case r.Error == "skipped (fail-fast)" || r.Error == "cancelled":
			summary.Skipped++
		default:
			summary.Failed++
			passed = false
		}
	}

	// If any test failed (not just skipped), the whole run fails
	if summary.Failed > 0 {
		passed = false
	}

	return &Result{
		Name:      e.config.Name,
		Timestamp: time.Now(),
		Duration:  duration,
		Passed:    passed,
		Summary:   summary,
		Tests:     results,
	}
}

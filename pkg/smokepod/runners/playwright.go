package runners

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/peteretelej/smokepod/pkg/smokepod"
)

// PlaywrightRunner executes Playwright tests in a container.
type PlaywrightRunner struct {
	container *smokepod.Container
}

// NewPlaywrightRunner creates a new Playwright test runner.
func NewPlaywrightRunner(container *smokepod.Container) *PlaywrightRunner {
	return &PlaywrightRunner{container: container}
}

// PlaywrightResult contains the parsed results from a Playwright run.
type PlaywrightResult struct {
	Passed   bool
	Duration time.Duration
	Total    int
	PassedN  int
	FailedN  int
	Skipped  int
	Suites   []SuiteResult
	RawJSON  string // original JSON for debugging
}

// SuiteResult represents results for a test suite.
type SuiteResult struct {
	Name  string
	File  string
	Specs []SpecResult
}

// SpecResult represents the result of a single spec.
type SpecResult struct {
	Name   string
	Passed bool
	Error  string
}

// Run executes Playwright tests and returns the results.
func (r *PlaywrightRunner) Run(ctx context.Context, args []string) (*PlaywrightResult, error) {
	// Build command: npx playwright test --reporter=json
	cmd := []string{"npx", "playwright", "test", "--reporter=json"}
	cmd = append(cmd, args...)

	// Execute in container
	execResult, err := r.container.Exec(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("executing playwright: %w", err)
	}

	// Parse JSON output from stdout
	result, err := ParsePlaywrightOutput(execResult.Stdout)
	if err != nil {
		// Return partial result with error info
		return &PlaywrightResult{
			Passed:  false,
			RawJSON: execResult.Stdout,
		}, fmt.Errorf("parsing playwright output: %w", err)
	}

	result.RawJSON = execResult.Stdout
	return result, nil
}

// ParsePlaywrightOutput parses Playwright JSON reporter output.
func ParsePlaywrightOutput(jsonStr string) (*PlaywrightResult, error) {
	if jsonStr == "" {
		return &PlaywrightResult{
			Passed: true,
		}, nil
	}

	var output PlaywrightOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	result := &PlaywrightResult{
		Total:    output.Stats.Total,
		PassedN:  output.Stats.Passed,
		FailedN:  output.Stats.Failed,
		Skipped:  output.Stats.Skipped,
		Duration: time.Duration(output.Stats.Duration) * time.Millisecond,
		Passed:   output.Stats.Failed == 0,
	}

	// Extract suite results
	result.Suites = extractSuites(output.Suites)

	return result, nil
}

// extractSuites recursively extracts suite results, handling nested suites.
func extractSuites(suites []PlaywrightSuite) []SuiteResult {
	var results []SuiteResult

	for _, suite := range suites {
		sr := SuiteResult{
			Name: suite.Title,
			File: suite.File,
		}

		// Extract specs from this suite
		for _, spec := range suite.Specs {
			specResult := SpecResult{
				Name:   spec.Title,
				Passed: spec.OK,
			}

			// Find first error from tests if spec failed
			if !spec.OK {
				for _, test := range spec.Tests {
					if test.Error != nil && test.Error.Message != "" {
						specResult.Error = test.Error.Message
						break
					}
				}
			}

			sr.Specs = append(sr.Specs, specResult)
		}

		// Only add suite if it has specs
		if len(sr.Specs) > 0 {
			results = append(results, sr)
		}

		// Process nested suites recursively
		nested := extractSuites(suite.Suites)
		results = append(results, nested...)
	}

	return results
}

// ToTestResult converts PlaywrightResult to the standard TestResult format.
func (r *PlaywrightResult) ToTestResult(name string) smokepod.TestResult {
	return smokepod.TestResult{
		Name:     name,
		Type:     "playwright",
		Passed:   r.Passed,
		Duration: r.Duration,
	}
}

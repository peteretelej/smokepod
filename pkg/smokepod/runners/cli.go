// Package runners provides test runners for different test types.
package runners

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/peteretelej/smokepod/internal/testfile"
	"github.com/peteretelej/smokepod/internal/whitespace"
)

// CLIRunner executes CLI tests in a container.
type CLIRunner struct {
	target Target
}

// NewCLIRunner creates a new CLI test runner.
func NewCLIRunner(target Target) *CLIRunner {
	return &CLIRunner{target: target}
}

// Run executes all commands in a section and returns results.
func (r *CLIRunner) Run(ctx context.Context, section *testfile.Section) (*SectionResult, error) {
	result := &SectionResult{
		Name:   section.Name,
		Passed: true,
	}

	for _, cmd := range section.Commands {
		cmdResult := r.runCommand(ctx, cmd)
		result.Commands = append(result.Commands, cmdResult)
		if !cmdResult.Passed {
			result.Passed = false
		}
	}

	return result, nil
}

func (r *CLIRunner) runCommand(ctx context.Context, cmd testfile.Command) CommandResult {
	result := CommandResult{
		Command: cmd.Cmd,
		Line:    cmd.Line,
		Passed:  true,
	}

	// Build expected output string for reporting
	var expectedLines []string
	for _, exp := range cmd.Expected {
		expectedLines = append(expectedLines, exp.Text)
	}
	result.Expected = strings.Join(expectedLines, "\n")

	execResult, err := r.target.Exec(ctx, cmd.Cmd)
	if err != nil {
		result.Passed = false
		result.Error = fmt.Sprintf("execution error: %v", err)
		return result
	}

	result.Actual = strings.TrimRight(execResult.Stdout, "\n")

	// Check exit code
	if execResult.ExitCode != cmd.ExitCode {
		result.Passed = false
		result.Error = fmt.Sprintf("exit code: got %d, want %d", execResult.ExitCode, cmd.ExitCode)
		return result
	}

	// Separate expectations into stdout and stderr groups
	var stdoutExpected, stderrExpected []testfile.Expect
	for _, exp := range cmd.Expected {
		if exp.IsStderr {
			stderrExpected = append(stderrExpected, exp)
		} else {
			stdoutExpected = append(stdoutExpected, exp)
		}
	}

	// Compare stdout expectations
	if len(stdoutExpected) > 0 {
		err, wsDiff := r.compareOutput(stdoutExpected, strings.TrimRight(execResult.Stdout, "\n"))
		if err != nil {
			result.Passed = false
			result.WhitespaceDiff = wsDiff
			result.Error = fmt.Sprintf("stdout: %s", err.Error())
			return result
		}
		result.WhitespaceDiff = wsDiff
	}

	// Compare stderr expectations
	if len(stderrExpected) > 0 {
		err, wsDiff := r.compareOutput(stderrExpected, strings.TrimRight(execResult.Stderr, "\n"))
		if err != nil {
			result.Passed = false
			result.WhitespaceDiff = result.WhitespaceDiff || wsDiff
			result.Error = fmt.Sprintf("stderr: %s", err.Error())
			return result
		}
		result.WhitespaceDiff = result.WhitespaceDiff || wsDiff
	}

	return result
}

func (r *CLIRunner) compareOutput(expected []testfile.Expect, actual string) (error, bool) {
	actualLines := strings.Split(actual, "\n")

	// Handle empty actual output
	if actual == "" {
		actualLines = []string{}
	}

	if len(actualLines) != len(expected) {
		diff, wsDiff := formatDiff(expected, actualLines)
		return fmt.Errorf("line count: got %d, want %d\n%s",
			len(actualLines), len(expected), diff), wsDiff
	}

	hasWSDiff := false
	for i, exp := range expected {
		actualLine := actualLines[i]
		if exp.IsRegex {
			matched, err := regexp.MatchString(exp.Text, actualLine)
			if err != nil {
				return fmt.Errorf("line %d: invalid regex %q: %v", exp.Line, exp.Text, err), false
			}
			if !matched {
				return fmt.Errorf("line %d: regex mismatch\n  pattern: %s\n  actual:  %s",
					exp.Line, exp.Text, actualLine), false
			}
		} else {
			if actualLine != exp.Text {
				if whitespace.IsWhitespaceDiff(exp.Text, actualLine) {
					hasWSDiff = true
					return fmt.Errorf("line %d: mismatch\n  want: %s\n  got:  %s",
						exp.Line, whitespace.RenderWhitespace(exp.Text), whitespace.RenderWhitespace(actualLine)), true
				}
				return fmt.Errorf("line %d: mismatch\n  want: %s\n  got:  %s",
					exp.Line, exp.Text, actualLine), false
			}
		}
	}

	return nil, hasWSDiff
}

func expectSuffix(exp testfile.Expect) string {
	var parts []string
	if exp.IsStderr {
		parts = append(parts, "stderr")
	}
	if exp.IsRegex {
		parts = append(parts, "re")
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ",") + ")"
}

func formatDiff(expected []testfile.Expect, actual []string) (string, bool) {
	var b strings.Builder
	hasWSDiff := false
	b.WriteString("--- expected\n+++ actual\n")

	maxLen := len(expected)
	if len(actual) > maxLen {
		maxLen = len(actual)
	}

	for i := 0; i < maxLen; i++ {
		if i < len(expected) && i < len(actual) {
			suffix := expectSuffix(expected[i])
			if !expected[i].IsRegex && whitespace.IsWhitespaceDiff(expected[i].Text, actual[i]) {
				hasWSDiff = true
				b.WriteString(fmt.Sprintf("- %s%s\n", whitespace.RenderWhitespace(expected[i].Text), suffix))
				b.WriteString(fmt.Sprintf("+ %s\n", whitespace.RenderWhitespace(actual[i])))
			} else {
				b.WriteString(fmt.Sprintf("- %s%s\n", expected[i].Text, suffix))
				b.WriteString(fmt.Sprintf("+ %s\n", actual[i]))
			}
		} else if i < len(expected) {
			suffix := expectSuffix(expected[i])
			b.WriteString(fmt.Sprintf("- %s%s\n", expected[i].Text, suffix))
		} else {
			b.WriteString(fmt.Sprintf("+ %s\n", actual[i]))
		}
	}

	return b.String(), hasWSDiff
}

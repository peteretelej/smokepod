package smokepod

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/peteretelej/smokepod/internal/testfile"
	"github.com/peteretelej/smokepod/internal/whitespace"
)

type CompareResult struct {
	Matched        bool
	Diff           string
	ExitCode       int
	ExitMatched    bool
	WhitespaceDiff bool
}

func CompareOutput(expected, actual string) CompareResult {
	expectedLines := splitLines(expected)
	actualLines := splitLines(actual)

	if len(expectedLines) != len(actualLines) {
		diff, wsDiff := formatDiff(expectedLines, actualLines)
		return CompareResult{
			Matched:        false,
			Diff:           diff,
			WhitespaceDiff: wsDiff,
		}
	}

	for i, exp := range expectedLines {
		if exp != actualLines[i] {
			diff, wsDiff := formatDiff(expectedLines, actualLines)
			return CompareResult{
				Matched:        false,
				Diff:           diff,
				WhitespaceDiff: wsDiff,
			}
		}
	}

	return CompareResult{
		Matched: true,
		Diff:    "",
	}
}

// CompareOutputWithExpectations compares fixture output against actual output,
// using regex matching for lines that have IsRegex=true in expectations.
// If expectations is nil/empty or has no regex entries, falls back to CompareOutput.
func CompareOutputWithExpectations(fixture, actual string, expectations []testfile.Expect, isStderr bool) CompareResult {
	// Filter expectations to the target stream
	var filtered []testfile.Expect
	for _, exp := range expectations {
		if exp.IsStderr == isStderr {
			filtered = append(filtered, exp)
		}
	}

	// If no filtered expectations have regex, delegate to literal comparison
	hasRegex := false
	for _, exp := range filtered {
		if exp.IsRegex {
			hasRegex = true
			break
		}
	}
	if !hasRegex {
		return CompareOutput(fixture, actual)
	}

	fixtureLines := splitLines(fixture)
	actualLines := splitLines(actual)

	// Pre-compile regex patterns indexed by line position
	compiledByPos := make(map[int]*regexp.Regexp)
	for i, exp := range filtered {
		if exp.IsRegex {
			re, err := regexp.Compile(exp.Text)
			if err != nil {
				return CompareResult{
					Matched: false,
					Diff:    fmt.Sprintf("line %d: invalid regex %q: %v", i+1, exp.Text, err),
				}
			}
			compiledByPos[i] = re
		}
	}

	// Line count mismatch
	if len(fixtureLines) != len(actualLines) {
		diff, wsDiff := formatDiff(fixtureLines, actualLines)
		return CompareResult{
			Matched:        false,
			Diff:           diff,
			WhitespaceDiff: wsDiff,
		}
	}

	for i, fixtureLine := range fixtureLines {
		actualLine := actualLines[i]
		if re, ok := compiledByPos[i]; ok {
			if !re.MatchString(actualLine) {
				return CompareResult{
					Matched: false,
					Diff:    fmt.Sprintf("line %d: regex mismatch\n  pattern: %s\n  actual:  %s", i+1, re.String(), actualLine),
				}
			}
		} else if fixtureLine != actualLine {
			diff, wsDiff := formatDiff(fixtureLines, actualLines)
			return CompareResult{
				Matched:        false,
				Diff:           diff,
				WhitespaceDiff: wsDiff || whitespace.IsWhitespaceDiff(fixtureLine, actualLine),
			}
		}
	}

	return CompareResult{Matched: true}
}

func CompareExitCode(expected, actual int) bool {
	return expected == actual
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}

	lines := strings.Split(s, "\n")

	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

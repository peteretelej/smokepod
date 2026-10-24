package smokepod

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/peteretelej/smokepod/internal/normalize"
	"github.com/peteretelej/smokepod/internal/testfile"
	"github.com/peteretelej/smokepod/internal/whitespace"
)

type CompareResult struct {
	Matched        bool
	Diff           string
	ExitCode       int
	ExitMatched    bool
	WhitespaceDiff bool
	// FirstLine, FirstExpected and FirstActual describe the first differing
	// line pair when Matched is false, for compact change summaries.
	FirstLine     int // 1-based; 0 when no line pair applies (e.g. count mismatch)
	FirstExpected string
	FirstActual   string
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
				FirstLine:      i + 1,
				FirstExpected:  exp,
				FirstActual:    actualLines[i],
			}
		}
	}

	return CompareResult{
		Matched: true,
		Diff:    "",
	}
}

// CompareOutputWithExpectations compares fixture output against actual output,
// using regex, normalized, and JSON matching for lines that have the
// corresponding flag in expectations. Normalization filters are applied to
// both sides before any comparison. If expectations is nil/empty or has no
// tier flags, falls back to CompareOutput.
func CompareOutputWithExpectations(fixture, actual string, expectations []testfile.Expect, isStderr bool, filters []string) CompareResult {
	if len(filters) > 0 {
		fixture = normalize.Text(fixture, filters)
		actual = normalize.Text(actual, filters)
	}

	// Filter expectations to the target stream
	var filtered []testfile.Expect
	for _, exp := range expectations {
		if exp.IsStderr == isStderr {
			filtered = append(filtered, exp)
		}
	}

	// If no filtered expectations use a non-default tier, delegate to literal
	// comparison
	hasTiers := false
	for _, exp := range filtered {
		if exp.IsRegex || exp.IsJSON || exp.IsNorm {
			hasTiers = true
			break
		}
	}
	if !hasTiers {
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
					Matched:       false,
					Diff:          fmt.Sprintf("line %d: regex mismatch\n  pattern: %s\n  actual:  %s", i+1, re.String(), actualLine),
					FirstLine:     i + 1,
					FirstExpected: fixtureLine,
					FirstActual:   actualLine,
				}
			}
			continue
		}
		if i < len(filtered) && filtered[i].IsJSON {
			matched, err := jsonEqual(fixtureLine, actualLine)
			if err != nil {
				return CompareResult{
					Matched: false,
					Diff:    fmt.Sprintf("line %d: %v", i+1, err),
				}
			}
			if !matched {
				return CompareResult{
					Matched:       false,
					Diff:          fmt.Sprintf("line %d: JSON mismatch (compared structurally, key order independent)", i+1),
					FirstLine:     i + 1,
					FirstExpected: fixtureLine,
					FirstActual:   actualLine,
				}
			}
			continue
		}
		if i < len(filtered) && filtered[i].IsNorm {
			if !normEqual(fixtureLine, actualLine) {
				return CompareResult{
					Matched:       false,
					Diff:          fmt.Sprintf("line %d: normalized mismatch (whitespace- and case-insensitive)", i+1),
					FirstLine:     i + 1,
					FirstExpected: fixtureLine,
					FirstActual:   actualLine,
				}
			}
			continue
		}
		if fixtureLine != actualLine {
			diff, wsDiff := formatDiff(fixtureLines, actualLines)
			return CompareResult{
				Matched:        false,
				Diff:           diff,
				WhitespaceDiff: wsDiff || whitespace.IsWhitespaceDiff(fixtureLine, actualLine),
				FirstLine:      i + 1,
				FirstExpected:  fixtureLine,
				FirstActual:    actualLine,
			}
		}
	}

	return CompareResult{Matched: true}
}

// normEqual compares two lines ignoring whitespace runs and letter case.
func normEqual(a, b string) bool {
	return strings.EqualFold(normalize.Collapse(a), normalize.Collapse(b))
}

// jsonEqual compares two lines as structurally equal JSON: object key order
// is independent, array order is significant. Returns an error naming the
// offending side when either line does not parse as JSON.
func jsonEqual(expected, actual string) (bool, error) {
	var expectedVal, actualVal any
	if err := json.Unmarshal([]byte(expected), &expectedVal); err != nil {
		return false, fmt.Errorf("expected line is not valid JSON: %s", jsonErrText(err))
	}
	if err := json.Unmarshal([]byte(actual), &actualVal); err != nil {
		return false, fmt.Errorf("actual line is not valid JSON: %s", jsonErrText(err))
	}
	return reflect.DeepEqual(expectedVal, actualVal), nil
}

// jsonErrText shortens encoding/json parse errors to their message without
// the structurally-aware offset detail.
func jsonErrText(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 {
		msg = msg[i+2:]
	}
	return msg
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

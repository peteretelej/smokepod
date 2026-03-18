package smokepod

import (
	"strings"
	"testing"

	"github.com/peteretelej/smokepod/internal/testfile"
)

func TestCompareOutput_Match(t *testing.T) {
	expected := "hello\nworld\n"
	actual := "hello\nworld\n"

	result := CompareOutput(expected, actual)

	if !result.Matched {
		t.Errorf("Expected match, got diff:\n%s", result.Diff)
	}
}

func TestCompareOutput_Mismatch(t *testing.T) {
	expected := "hello\nworld\n"
	actual := "hello\nuniverse\n"

	result := CompareOutput(expected, actual)

	if result.Matched {
		t.Error("Expected mismatch, got match")
	}

	if !strings.Contains(result.Diff, "-world") {
		t.Errorf("Diff should contain '-world', got:\n%s", result.Diff)
	}

	if !strings.Contains(result.Diff, "+universe") {
		t.Errorf("Diff should contain '+universe', got:\n%s", result.Diff)
	}
}

func TestCompareOutput_LineCountMismatch(t *testing.T) {
	expected := "line1\nline2\n"
	actual := "line1\n"

	result := CompareOutput(expected, actual)

	if result.Matched {
		t.Error("Expected mismatch due to line count")
	}
}

func TestCompareOutput_Empty(t *testing.T) {
	result := CompareOutput("", "")

	if !result.Matched {
		t.Error("Empty strings should match")
	}
}

func TestCompareExitCode_Match(t *testing.T) {
	if !CompareExitCode(0, 0) {
		t.Error("Exit code 0 should match 0")
	}

	if !CompareExitCode(42, 42) {
		t.Error("Exit code 42 should match 42")
	}
}

func TestCompareExitCode_Mismatch(t *testing.T) {
	if CompareExitCode(0, 1) {
		t.Error("Exit code 0 should not match 1")
	}

	if CompareExitCode(42, 0) {
		t.Error("Exit code 42 should not match 0")
	}
}

func TestFormatDiff(t *testing.T) {
	expected := []string{"line1", "line2", "line3"}
	actual := []string{"line1", "different", "line3"}

	diff, wsDiff := formatDiff(expected, actual)

	if wsDiff {
		t.Error("Content-only diff should not report whitespace diff")
	}

	if !strings.Contains(diff, "--- expected") {
		t.Error("Diff should contain expected header")
	}

	if !strings.Contains(diff, "+++ actual") {
		t.Error("Diff should contain actual header")
	}

	if !strings.Contains(diff, "-line2") {
		t.Error("Diff should show removed line")
	}

	if !strings.Contains(diff, "+different") {
		t.Error("Diff should show added line")
	}
}

func TestCompareOutput_WhitespaceMismatch(t *testing.T) {
	expected := "hello \n"
	actual := "hello\n"

	result := CompareOutput(expected, actual)

	if result.Matched {
		t.Error("Expected mismatch for trailing space difference")
	}
	if !result.WhitespaceDiff {
		t.Error("Expected WhitespaceDiff=true for trailing space difference")
	}
	if !strings.Contains(result.Diff, "\u00B7") {
		t.Errorf("Expected diff to contain · (space marker), got:\n%s", result.Diff)
	}
}

func TestCompareOutput_ContentMismatch_NoWhitespaceFlag(t *testing.T) {
	expected := "hello\n"
	actual := "goodbye\n"

	result := CompareOutput(expected, actual)

	if result.Matched {
		t.Error("Expected mismatch for content difference")
	}
	if result.WhitespaceDiff {
		t.Error("Expected WhitespaceDiff=false for content-only difference")
	}
	if strings.Contains(result.Diff, "\u00B7") {
		t.Errorf("Expected no · marker for content diff, got:\n%s", result.Diff)
	}
}

func TestCompareOutput_CarriageReturn(t *testing.T) {
	expected := "hello\r\n"
	actual := "hello\n"

	result := CompareOutput(expected, actual)

	if result.Matched {
		t.Error("Expected mismatch for CR difference")
	}
	if !result.WhitespaceDiff {
		t.Error("Expected WhitespaceDiff=true for CR difference")
	}
	if !strings.Contains(result.Diff, "\u00AC") {
		t.Errorf("Expected diff to contain ¬ (CR marker), got:\n%s", result.Diff)
	}
}

func TestCompareOutput_TabVsSpaces(t *testing.T) {
	expected := "\thello\n"
	actual := "  hello\n"

	result := CompareOutput(expected, actual)

	if result.Matched {
		t.Error("Expected mismatch for tab vs spaces")
	}
	if !result.WhitespaceDiff {
		t.Error("Expected WhitespaceDiff=true for tab vs spaces difference")
	}
	if !strings.Contains(result.Diff, "\u2192") {
		t.Errorf("Expected diff to contain → (tab marker), got:\n%s", result.Diff)
	}
	if !strings.Contains(result.Diff, "\u00B7") {
		t.Errorf("Expected diff to contain · (space marker), got:\n%s", result.Diff)
	}
}

func TestCompareOutputWithExpectations_NilExpectations(t *testing.T) {
	result := CompareOutputWithExpectations("hello\nworld\n", "hello\nworld\n", nil, false)
	if !result.Matched {
		t.Errorf("nil expectations should match identical strings, got diff:\n%s", result.Diff)
	}

	result = CompareOutputWithExpectations("hello\n", "goodbye\n", nil, false)
	if result.Matched {
		t.Error("nil expectations should detect mismatches")
	}
}

func TestCompareOutputWithExpectations_RegexMatchPasses(t *testing.T) {
	expectations := []testfile.Expect{
		{Text: `\d{4}-\d{2}-\d{2}`, IsRegex: true},
	}
	result := CompareOutputWithExpectations("2026-03-18\n", "2026-03-19\n", expectations, false)
	if !result.Matched {
		t.Errorf("regex should match date pattern, got diff:\n%s", result.Diff)
	}
}

func TestCompareOutputWithExpectations_RegexMatchFails(t *testing.T) {
	expectations := []testfile.Expect{
		{Text: `\d{4}-\d{2}-\d{2}`, IsRegex: true},
	}
	result := CompareOutputWithExpectations("2026-03-18\n", "not-a-date\n", expectations, false)
	if result.Matched {
		t.Error("regex should not match non-date string")
	}
	if !strings.Contains(result.Diff, "regex mismatch") {
		t.Errorf("expected regex mismatch message, got:\n%s", result.Diff)
	}
}

func TestCompareOutputWithExpectations_MixedRegexAndLiteral(t *testing.T) {
	expectations := []testfile.Expect{
		{Text: "header"},
		{Text: `\d+`, IsRegex: true},
		{Text: "footer"},
	}
	result := CompareOutputWithExpectations(
		"header\n999\nfooter\n",
		"header\n42\nfooter\n",
		expectations, false)
	if !result.Matched {
		t.Errorf("mixed regex/literal should match, got diff:\n%s", result.Diff)
	}

	result = CompareOutputWithExpectations(
		"header\n999\nfooter\n",
		"HEADER\n42\nfooter\n",
		expectations, false)
	if result.Matched {
		t.Error("literal line mismatch should fail")
	}
}

func TestCompareOutputWithExpectations_InvalidRegex(t *testing.T) {
	expectations := []testfile.Expect{
		{Text: "[invalid", IsRegex: true},
	}
	result := CompareOutputWithExpectations("anything\n", "anything\n", expectations, false)
	if result.Matched {
		t.Error("invalid regex should cause failure")
	}
	if !strings.Contains(result.Diff, "invalid regex") {
		t.Errorf("expected invalid regex message, got:\n%s", result.Diff)
	}
}

func TestCompareOutputWithExpectations_FewerExpectationsThanLines(t *testing.T) {
	// 2 expectations for 5 output lines. Expectations are matched by position
	// in the filtered slice: pos 0 gets regex, pos 1 gets literal check.
	// Lines beyond the expectation count (3-5) compare against the fixture.
	expectations := []testfile.Expect{
		{Text: `\d+`, IsRegex: true},
		{Text: "literal"},
	}
	result := CompareOutputWithExpectations(
		"100\nliteral\nthird\nfourth\nfifth\n",
		"999\nliteral\nthird\nfourth\nfifth\n",
		expectations, false)
	if !result.Matched {
		t.Errorf("should match (regex at pos 0, fixture comparison for lines beyond expectations), got diff:\n%s", result.Diff)
	}
}

func TestCompareOutputWithExpectations_StderrFiltering(t *testing.T) {
	expectations := []testfile.Expect{
		{Text: "stdout-line"},
		{Text: `error: .*`, IsRegex: true, IsStderr: true},
	}

	// For stdout: only non-stderr expectations, no regex -> literal compare
	result := CompareOutputWithExpectations("hello\n", "hello\n", expectations, false)
	if !result.Matched {
		t.Errorf("stdout should match literally, got diff:\n%s", result.Diff)
	}

	// For stderr: only stderr expectations, regex match
	result = CompareOutputWithExpectations("error: old\n", "error: new thing\n", expectations, true)
	if !result.Matched {
		t.Errorf("stderr regex should match, got diff:\n%s", result.Diff)
	}
}
